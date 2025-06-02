package executor

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"go.uber.org/zap"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/logger"
)

type CommandConfig struct {
	WorkspaceDir       string // host path to bind to /workspace
	InputDirName       string
	OutputDirName      string
	ExecResultFileName string
	DockerImage        string
	TimeLimits         []int
	MemoryLimits       []int
}

type ExecutionResult struct {
	ExitCode int
	ExecTime float64
}

type DockerExecutor struct {
	logger         *zap.SugaredLogger
	cli            *client.Client
	jobsDataVolume string
}

func NewDockerExecutor(volume string) (*DockerExecutor, error) {
	logger := logger.NewNamedLogger("docker-executor")
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &DockerExecutor{cli: cli, jobsDataVolume: volume, logger: logger}, nil
}

func (d *DockerExecutor) ExecuteCommand(
	solutionPath, messageID string, cfg CommandConfig,
) error {
	ctx := context.Background()

	// Build the run_tests invocation
	runCmd := d.buildRunCommand(solutionPath)

	// Build environment variables
	env := d.buildEnvironmentVariables(cfg)

	// Container configuration
	containerCfg := d.buildContainerConfig(cfg, runCmd, env)

	// Host configuration
	hostCfg := d.buildHostConfig(cfg)

	// Prepare the Docker image
	if err := d.PrepareImageIfNotPresent(ctx, cfg); err != nil {
		d.logger.Errorf("Failed to prepare image %s: %s [MsgID: %s]", cfg.DockerImage, err, messageID)
		return err
	}

	// Create and start the container
	resp, err := d.createAndStartContainer(ctx, containerCfg, hostCfg, messageID)
	if err != nil {
		return err
	}

	// Wait for the container to exit
	return d.waitForContainer(ctx, resp.ID, messageID)
}

func (d *DockerExecutor) buildRunCommand(solutionPath string) string {
	bin := filepath.Base(solutionPath) // e.g. "solution"
	return fmt.Sprintf("run_tests.sh ./%s", bin)
}

func (d *DockerExecutor) buildEnvironmentVariables(cfg CommandConfig) []string {
	const minMemKB = 128 * 1024 // 128 MB minimum per-test
	timeEnv := make([]string, len(cfg.TimeLimits))
	for i, s := range cfg.TimeLimits {
		timeEnv[i] = strconv.Itoa(s)
	}
	memEnv := make([]string, len(cfg.MemoryLimits))
	for i, kb := range cfg.MemoryLimits {
		if kb < minMemKB {
			kb = minMemKB
		}
		memEnv[i] = strconv.Itoa(kb)
	}
	return []string{
		"TIME_LIMITS=" + strings.Join(timeEnv, " "),
		"MEM_LIMITS=" + strings.Join(memEnv, " "),
		"INPUT_DIR=" + cfg.InputDirName,
		"OUTPUT_DIR=" + cfg.OutputDirName,
		"EXEC_RESULT_FILE=" + cfg.ExecResultFileName,
	}
}

func (d *DockerExecutor) buildContainerConfig(cfg CommandConfig, runCmd string, env []string) *container.Config {
	stopTimeout := int(2)
	return &container.Config{
		Image:       cfg.DockerImage,
		Cmd:         []string{"bash", "-lc", runCmd},
		WorkingDir:  cfg.WorkspaceDir,
		Env:         env,
		User:        "0:0",
		StopTimeout: &stopTimeout,
		StopSignal:  "SIGKILL",
	}
}

func (d *DockerExecutor) buildHostConfig(cfg CommandConfig) *container.HostConfig {
	return &container.HostConfig{
		AutoRemove:  true,
		NetworkMode: container.NetworkMode("none"),
		Resources: container.Resources{
			PidsLimit: func(v int64) *int64 { return &v }(64),
			Memory:    int64(maxIntSlice(cfg.MemoryLimits, 6*1024)) * 1024,
			CPUPeriod: 100_000,
			CPUQuota:  100_000,
		},
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: d.jobsDataVolume,
			Target: "/tmp",
		}},
		SecurityOpt:  []string{"no-new-privileges"},
		CgroupnsMode: container.CgroupnsModePrivate,
		IpcMode:      container.IpcMode("private"),
	}
}

func (d *DockerExecutor) createAndStartContainer(
	ctx context.Context, containerCfg *container.Config, hostCfg *container.HostConfig, messageID string,
) (*container.CreateResponse, error) {
	name := fmt.Sprintf("submission-%s", messageID)
	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
	if err != nil {
		d.logger.Errorf("Create failed: %s [MsgID: %s]", err, messageID)
		return nil, err
	}
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		d.logger.Errorf("Start failed: %s [MsgID: %s]", err, messageID)
		return nil, err
	}
	d.logger.Infof("Started container %s [MsgID: %s]", resp.ID, messageID)
	return &resp, nil
}

func (d *DockerExecutor) waitForContainer(
	ctx context.Context, containerID, messageID string,
) error {
	timeout := time.Duration(constants.ContainerMaxRunTime) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	statusCh, errCh := d.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var exitCode int
	select {
	case err := <-errCh:
		d.logger.Errorf("Wait error: %s [MsgID: %s]", err, messageID)
		return err
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
	case <-timer.C:
		d.logger.Errorf("Container runtime exceeded %d seconds, killing... [MsgID: %s]",
			constants.ContainerMaxRunTime, messageID)
		err := d.cli.ContainerKill(ctx, containerID, "SIGKILL")
		if err != nil {
			d.logger.Errorf("Failed to kill container: %s [MsgID: %s]", err, messageID)
		}
		return errors.ErrContainerTimeout
	}

	if exitCode == 1 {
		d.logger.Errorf("Internal error [MsgID: %s]", messageID)
		return errors.ErrContainerFailed
	}

	d.logger.Infof("Finished container %s [MsgID: %s]", containerID, messageID)
	return nil
}

// Helper function to prepare the Docker image by pulling it if not present.
func (d *DockerExecutor) PrepareImageIfNotPresent(ctx context.Context, cfg CommandConfig) error {
	// Check if the image is already present
	_, err := d.cli.ImageInspect(ctx, cfg.DockerImage)
	if err == nil {
		return nil
	}
	if !client.IsErrNotFound(err) {
		return err
	}

	d.logger.Infof("Image %s not found, pulling...", cfg.DockerImage)
	reader, err := d.cli.ImagePull(ctx, cfg.DockerImage, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	return err
}

// maxIntSlice returns the larger of the values in a, or min if all are smaller.
func maxIntSlice(a []int, minKB int) int {
	m := minKB
	for _, v := range a {
		if v > m {
			m = v
		}
	}
	return m
}

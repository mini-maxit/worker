package executor

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"

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
	cli            *client.Client
	jobsDataVolume string
}

func NewDockerExecutor(volume string) (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &DockerExecutor{cli: cli, jobsDataVolume: volume}, nil
}

func (d *DockerExecutor) ExecuteCommand(
	solutionPath, messageID string, cfg CommandConfig,
) error {
	log := logger.NewNamedLogger("docker-executor")
	ctx := context.Background()

	// --- 1) Build the run_tests invocation ---
	bin := filepath.Base(solutionPath) // e.g. "solution"
	runCmd := fmt.Sprintf("run_tests.sh ./%s", bin)

	// --- 2) Build the TIME_LIMITS and MEM_LIMITS env lines ---
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
	env := []string{
		"TIME_LIMITS=" + strings.Join(timeEnv, " "),
		"MEM_LIMITS=" + strings.Join(memEnv, " "),
		"INPUT_DIR=" + cfg.InputDirName,
		"OUTPUT_DIR=" + cfg.OutputDirName,
		"EXEC_RESULT_FILE=" + cfg.ExecResultFileName,
	}

	// --- 3) Container config: run from cfg.WorkspaceDir (under /tmp) ---
	stopTimeout := int(2)
	containerCfg := &container.Config{
		Image:       cfg.DockerImage,
		Cmd:         []string{"bash", "-lc", runCmd},
		WorkingDir:  cfg.WorkspaceDir,
		Env:         env,
		User:        "0:0",
		StopTimeout: &stopTimeout,
		StopSignal:  "SIGKILL",
	}

	// --- 4) Host config: mount the named volume over /tmp ---
	hostCfg := &container.HostConfig{
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

	// --- 5) Create & Start ---
	name := fmt.Sprintf("submission-%s", messageID)
	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
	if err != nil {
		log.Errorf("Create failed: %s [MsgID: %s]", err, messageID)
		return err
	}
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Errorf("Start failed: %s [MsgID: %s]", err, messageID)
		return err
	}
	log.Infof("Started container %s [MsgID: %s]", resp.ID, messageID)

	// --- 6) Wait for it to exit ---
	timeout := time.Duration(constants.ContainerMaxRunTime) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	statusCh, errCh := d.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int
	select {
	case err := <-errCh:
		log.Errorf("Wait error: %s [MsgID: %s]", err, messageID)
		return err
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
	case <-timer.C:
		log.Errorf("Container runtime exceeded %d seconds, killing... [MsgID: %s]", constants.ContainerMaxRunTime, messageID)
		err := d.cli.ContainerKill(ctx, resp.ID, "SIGKILL")
		if err != nil {
			log.Errorf("Failed to kill container: %s [MsgID: %s]", err, messageID)
		}
		return errors.ErrContainerTimeout
	}

	if exitCode == 1 {
		log.Errorf("Internal error [MsgID: %s]", messageID)
		return errors.ErrContainerFailed
	}

	log.Infof("Finished container %s [MsgID: %s]", resp.ID, messageID)
	return nil
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

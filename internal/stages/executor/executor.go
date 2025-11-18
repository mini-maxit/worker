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

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
)

type CommandConfig struct {
	MessageID       string
	DirConfig       *packager.TaskDirConfig
	LanguageType    languages.LanguageType
	LanguageVersion string
	TestCases       []messages.TestCase
}

type ExecutionResult struct {
	ExitCode int
	ExecTime float64
	PeakMem  int64
}

type Executor interface {
	ExecuteCommand(cfg CommandConfig) error
}

type executor struct {
	logger         *zap.SugaredLogger
	cli            *client.Client
	jobsDataVolume string
}

func NewExecutor(volume string, cli *client.Client) Executor {
	logger := logger.NewNamedLogger("docker-executor")

	return &executor{cli: cli, jobsDataVolume: volume, logger: logger}
}

func (d *executor) ExecuteCommand(
	cfg CommandConfig,
) error {
	// Create a context with a timeout to avoid indefinite execution
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(constants.ContainerMaxRunTime)*time.Second)
	defer cancel()

	// Build the run_tests invocation
	runCmd := d.buildRunCommand(cfg.DirConfig.UserExecFilePath)

	// Build environment variables
	env := d.buildEnvironmentVariables(cfg)

	// Docker image name
	dockerImage, err := cfg.LanguageType.GetDockerImage(cfg.LanguageVersion)
	if err != nil {
		d.logger.Errorf("Failed to get Docker image for language %s version %s: %s [MsgID: %s]",
			cfg.LanguageType, cfg.LanguageVersion, err, cfg.MessageID)
		return err
	}

	// Container configuration
	containerCfg := d.buildContainerConfig(cfg.DirConfig.PackageDirPath, dockerImage, runCmd, env)

	// Host configuration
	hostCfg := d.buildHostConfig(cfg)

	// Prepare the Docker image
	if err := d.prepareImageIfNotPresent(ctx, dockerImage); err != nil {
		d.logger.Errorf("Failed to prepare image %s: %s [MsgID: %s]", dockerImage, err, cfg.MessageID)
		return err
	}

	// Create and start the container
	resp, err := d.createAndStartContainer(ctx, containerCfg, hostCfg, cfg.MessageID)
	if err != nil {
		return err
	}

	// Wait for the container to exit
	return d.waitForContainer(ctx, resp.ID, cfg.MessageID)
}

func (d *executor) buildRunCommand(absoluteSolutionPath string) string {
	bin := filepath.Base(absoluteSolutionPath) // e.g. "solution"
	return fmt.Sprintf("%s ./%s", constants.DockerTestScript, bin)
}

func (d *executor) buildEnvironmentVariables(cfg CommandConfig) []string {
	timeEnv := make([]string, len(cfg.TestCases))
	memEnv := make([]string, len(cfg.TestCases))
	for i, tc := range cfg.TestCases {
		timeEnv[i] = strconv.FormatInt(tc.TimeLimitMs, 10)
		memEnv[i] = strconv.FormatInt(tc.MemoryLimitKB, 10)
	}

	inputFilePaths := make([]string, len(cfg.TestCases))
	userOutputFilePaths := make([]string, len(cfg.TestCases))
	userErrorFilePaths := make([]string, len(cfg.TestCases))
	userExecResultFilePaths := make([]string, len(cfg.TestCases))

	// Build paths for each test case
	for i, tc := range cfg.TestCases {
		inputFilePaths[i] = filepath.Join(
			cfg.DirConfig.InputDirPath,
			filepath.Base(tc.InputFile.Path))

		userOutputFilePaths[i] = filepath.Join(
			cfg.DirConfig.UserOutputDirPath,
			filepath.Base(tc.StdOutResult.Path))

		userErrorFilePaths[i] = filepath.Join(
			cfg.DirConfig.UserErrorDirPath,
			filepath.Base(tc.StdErrResult.Path))

		userExecResultFilePaths[i] = filepath.Join(
			cfg.DirConfig.UserExecResultDirPath,
			fmt.Sprintf("%d.%s", tc.Order, constants.ExecutionResultFileExt))
	}

	return []string{
		"TIME_LIMITS_MS=" + strings.Join(timeEnv, " "),
		"MEM_LIMITS_KB=" + strings.Join(memEnv, " "),
		"INPUT_FILES=" + strings.Join(inputFilePaths, " "),
		"USER_OUTPUT_FILES=" + strings.Join(userOutputFilePaths, " "),
		"USER_ERROR_FILES=" + strings.Join(userErrorFilePaths, " "),
		"USER_EXEC_RESULT_FILES=" + strings.Join(userExecResultFilePaths, " "),
	}
}

func (d *executor) buildContainerConfig(
	userPackageDirPath string,
	dockerImage string,
	runCmd string,
	env []string,
) *container.Config {
	stopTimeout := int(2)
	d.logger.Infof("Running command in container: %s", runCmd)
	d.logger.Infof("Workspace directory in container: %s", userPackageDirPath)
	return &container.Config{
		Image: dockerImage,
		// Cmd: 	   []string{"bash", "-c", "while true; do sleep 1000; done"},
		Cmd:         []string{"bash", "-lc", runCmd},
		WorkingDir:  userPackageDirPath,
		Env:         env,
		User:        "0:0",
		StopTimeout: &stopTimeout,
		StopSignal:  "SIGKILL",
	}
}

func (d *executor) buildHostConfig(cfg CommandConfig) *container.HostConfig {
	return &container.HostConfig{
		AutoRemove:  true,
		NetworkMode: container.NetworkMode("none"),
		Resources: container.Resources{
			PidsLimit: func(v int64) *int64 { return &v }(64),
			CPUPeriod: 100_000,
			CPUQuota:  100_000,
		},
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: d.jobsDataVolume,
			Target: cfg.DirConfig.TmpDirPath,
		}},
		SecurityOpt:  []string{"no-new-privileges"},
		CgroupnsMode: container.CgroupnsModePrivate,
		IpcMode:      container.IpcMode("private"),
	}
}

func (d *executor) createAndStartContainer(
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

func (d *executor) waitForContainer(
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

	if exitCode != 0 {
		d.logger.Errorf("Container exited with non-zero code %d [MsgID: %s]", exitCode, messageID)
		return errors.ErrContainerFailed
	}

	d.logger.Infof("Finished container %s [MsgID: %s]", containerID, messageID)
	return nil
}

// Helper function to prepare the Docker image by pulling it if not present.
func (d *executor) prepareImageIfNotPresent(ctx context.Context, dockerImage string) error {
	// Check if the image is already present
	_, err := d.cli.ImageInspect(ctx, dockerImage)
	if err == nil {
		return nil
	}
	if !client.IsErrNotFound(err) {
		return err
	}

	d.logger.Infof("Image %s not found, pulling...", dockerImage)
	reader, err := d.cli.ImagePull(ctx, dockerImage, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	return err
}

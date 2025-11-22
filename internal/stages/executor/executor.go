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

	"github.com/mini-maxit/worker/internal/config"
	"github.com/mini-maxit/worker/internal/docker"
	"go.uber.org/zap"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
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
}

type Executor interface {
	ExecuteCommand(cfg CommandConfig) error
}

type executor struct {
	logger *zap.SugaredLogger
	docker docker.DockerClient
	debug  config.ExecutorDebugConfig
}

func NewExecutor(dCli docker.DockerClient, debug config.ExecutorDebugConfig) Executor {
	logger := logger.NewNamedLogger("docker-executor")
	return &executor{docker: dCli, logger: logger, debug: debug}
}

func (d *executor) ExecuteCommand(
	cfg CommandConfig,
) error {
	maxRuntimeSec := int(constants.ContainerMaxRunTime)
	if d.debug.MaxRuntimeSec > 0 {
		maxRuntimeSec = d.debug.MaxRuntimeSec
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(maxRuntimeSec)*time.Second)
	defer cancel()

	// Build environment variables.
	env := d.buildEnvironmentVariables(cfg)

	// Docker image name.
	dockerImage, err := cfg.LanguageType.GetDockerImage(cfg.LanguageVersion)
	if err != nil {
		d.logger.Errorf("Failed to get Docker image for language %s version %s: %s [MsgID: %s]",
			cfg.LanguageType, cfg.LanguageVersion, err, cfg.MessageID)
		return err
	}
	if d.debug.DockerImage != "" {
		dockerImage = d.debug.DockerImage
		d.logger.Warnf("Executor debug: overriding docker image to %s [MsgID: %s]", dockerImage, cfg.MessageID)
	}

	// Container configuration.
	customRunCmd := d.debug.RunCmd
	if customRunCmd != "" {
		d.logger.Warnf("Executor debug: using custom run command: %s [MsgID: %s]", customRunCmd, cfg.MessageID)
	}
	containerCfg := d.buildContainerConfig(
		cfg.DirConfig.PackageDirPath,
		dockerImage,
		cfg.DirConfig.UserExecFilePath,
		env,
		customRunCmd,
	)

	// Host configuration.
	hostCfg := d.buildHostConfig(cfg)

	// Prepare the Docker image.
	if err := d.docker.EnsureImage(ctx, dockerImage); err != nil {
		d.logger.Errorf("Failed to prepare image %s: %s [MsgID: %s]", dockerImage, err, cfg.MessageID)
		return err
	}

	// Create and start the container.
	containerID, err := d.docker.CreateAndStartContainer(ctx, containerCfg, hostCfg, cfg.MessageID)
	if err != nil {
		d.logger.Errorf("Failed to create/start container: %s [MsgID: %s]", err, cfg.MessageID)
		return err
	}

	// Wait for the container to exit.
	waitTimeoutSec := maxRuntimeSec
	if d.debug.WaitTimeoutSec > 0 {
		waitTimeoutSec = d.debug.WaitTimeoutSec
	}

	return d.waitForContainer(ctx, containerID, cfg.MessageID, waitTimeoutSec)
}

func (d *executor) buildEnvironmentVariables(cfg CommandConfig) []string {
	const minMemKB int64 = 128 * 1024 // 128 MB minimum per-test
	timeEnv := make([]string, len(cfg.TestCases))
	memEnv := make([]string, len(cfg.TestCases))
	for i, tc := range cfg.TestCases {
		timeEnv[i] = strconv.FormatInt(tc.TimeLimitMs, 10)
		kb := tc.MemoryLimitKB
		if kb < minMemKB {
			kb = minMemKB
		}
		memEnv[i] = strconv.FormatInt(kb, 10)
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
		"TIME_LIMITS=" + strings.Join(timeEnv, " "),
		"MEM_LIMITS=" + strings.Join(memEnv, " "),
		"INPUT_FILES=" + strings.Join(inputFilePaths, " "),
		"USER_OUTPUT_FILES=" + strings.Join(userOutputFilePaths, " "),
		"USER_ERROR_FILES=" + strings.Join(userErrorFilePaths, " "),
		"USER_EXEC_RESULT_FILES=" + strings.Join(userExecResultFilePaths, " "),
	}
}

func (d *executor) buildContainerConfig(
	userPackageDirPath string,
	dockerImage string,
	absoluteSolutionPath string,
	env []string,
	customRunCmd string,
) *container.Config {
	stopTimeout := int(2)
	d.logger.Infof("Workspace directory in container: %s", userPackageDirPath)
	runCmd := customRunCmd
	if runCmd == "" {
		bin := filepath.Base(absoluteSolutionPath) // e.g. "solution"
		runCmd = constants.DockerTestScript + " ./" + bin
	}
	return &container.Config{
		Image:       dockerImage,
		Cmd:         []string{"bash", "-lc", runCmd},
		WorkingDir:  userPackageDirPath,
		Env:         env,
		User:        "0:0",
		StopTimeout: &stopTimeout,
		StopSignal:  "SIGKILL",
	}
}

func (d *executor) buildHostConfig(cfg CommandConfig) *container.HostConfig {
	memLimitsKb := make([]solution.Limit, len(cfg.TestCases))
	for i, tc := range cfg.TestCases {
		memLimitsKb[i] = solution.Limit{
			TimeMs:   tc.TimeLimitMs,
			MemoryKb: tc.MemoryLimitKB,
		}
	}

	return &container.HostConfig{
		AutoRemove:  true,
		NetworkMode: container.NetworkMode("none"),
		Resources: container.Resources{
			PidsLimit: func(v int64) *int64 { return &v }(64),
			Memory:    solution.MaxMemoryKBWithMinimum(memLimitsKb) * 1024,
			CPUPeriod: 100_000,
			CPUQuota:  100_000,
		},
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: d.docker.DataVolumeName(),
			Target: cfg.DirConfig.TmpDirPath,
		}},
		SecurityOpt:  []string{"no-new-privileges"},
		CgroupnsMode: container.CgroupnsModePrivate,
		IpcMode:      container.IpcMode("private"),
	}
}

func (d *executor) waitForContainer(
	ctx context.Context, containerID, messageID string,
	waitTimeoutSec int,
) error {
	timeout := time.Duration(waitTimeoutSec) * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	statusCh, errCh := d.docker.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
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
		err := d.docker.ContainerKill(ctx, containerID, "SIGKILL")
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

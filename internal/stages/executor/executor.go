package executor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"path/filepath"
	"regexp"

	"github.com/docker/docker/api/types/container"

	"github.com/mini-maxit/worker/internal/docker"
	"go.uber.org/zap"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	customErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/utils"
)

var containerNameRegex = regexp.MustCompile("[^a-zA-Z0-9_.-]")

type CommandConfig struct {
	MessageID         string
	DirConfig         *packager.TaskDirConfig
	LanguageType      languages.LanguageType
	LanguageVersion   string
	TestCases         []messages.TestCase
	SourceFilePath    string
	RequiresCompiling bool
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
	logger        *zap.SugaredLogger
	docker        docker.DockerClient
	maxRunTimeSec int
}

func NewExecutor(dCli docker.DockerClient, timeoutSec ...int) Executor {
	logger := logger.NewNamedLogger("docker-executor")
	t := constants.ContainerMaxRunTime
	if len(timeoutSec) > 0 && timeoutSec[0] > 0 {
		t = timeoutSec[0]
	}
	return &executor{docker: dCli, logger: logger, maxRunTimeSec: t}
}

func (d *executor) ExecuteCommand(
	cfg CommandConfig,
) error {
	d.logger.Infof("Starting execution  [MsgID: %s]", cfg.MessageID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(d.maxRunTimeSec)*time.Second)
	defer cancel()

	env, err := buildEnvironmentVariables(cfg)
	if err != nil {
		d.logger.Errorf("Failed to build environment variables: %s [MsgID: %s]", err, cfg.MessageID)
		return err
	}

	dockerImage, err := cfg.LanguageType.GetDockerImage(cfg.LanguageVersion)
	if err != nil {
		d.logger.Errorf("Failed to get Docker image for language %s version %s: %s [MsgID: %s]",
			cfg.LanguageType, cfg.LanguageVersion, err, cfg.MessageID)
		return err
	}

	containerCfg := buildContainerConfig(
		cfg.DirConfig.PackageDirPath,
		dockerImage,
		env,
	)

	hostCfg := buildHostConfig(cfg.TestCases)

	if err := d.docker.EnsureImage(ctx, dockerImage); err != nil {
		return err
	}

	containerName := SanitizeContainerName(cfg.MessageID)
	containerID, err := d.docker.CreateContainer(ctx, containerCfg, hostCfg, containerName)
	if err != nil {
		return err
	}

	// Ensure container cleanup
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		d.docker.ContainerRemove(cleanupCtx, containerID)
	}()

	d.logger.Infof("Copying package to container %s [MsgID: %s]", containerID, cfg.MessageID)
	excludes := []string{constants.OutputDirName}
	err = d.docker.CopyToContainerFiltered(
		ctx,
		containerID,
		cfg.DirConfig.PackageDirPath,
		cfg.DirConfig.TmpDirPath,
		excludes,
	)
	if err != nil {
		return err
	}

	if err := d.docker.StartContainer(ctx, containerID); err != nil {
		return err
	}

	waitErr := d.waitForContainer(ctx, containerID)
	if waitErr != nil {
		return waitErr
	}

	d.logger.Infof("Copying results from container %s [MsgID: %s]", containerID, cfg.MessageID)
	copyCtx, copyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer copyCancel()

	allowedDirs := []string{
		constants.UserOutputDirName,
		constants.UserErrorDirName,
		constants.UserDiffDirName,
		constants.UserExecResultDirName,
	}

	alwaysCopyFiles := []string{
		filepath.Base(cfg.DirConfig.CompileErrFilePath),
	}

	err = d.docker.CopyFromContainerFiltered(
		copyCtx,
		containerID,
		cfg.DirConfig.PackageDirPath,
		cfg.DirConfig.TmpDirPath,
		allowedDirs,
		alwaysCopyFiles,
		constants.MaxContainerOutputFileSize,
		len(cfg.TestCases),
	)
	if err != nil {
		return err
	}

	d.logger.Infof("Execution completed successfully for message ID %s", cfg.MessageID)
	return nil
}

func (d *executor) waitForContainer(
	ctx context.Context, containerID string,
) error {
	timeout := time.Duration(d.maxRunTimeSec) * time.Second
	exitCode, err := d.docker.WaitContainer(ctx, containerID, timeout)
	if err != nil {
		if errors.Is(err, customErr.ErrContainerTimeout) {
			d.docker.ContainerKill(ctx, containerID, "SIGKILL")
			return customErr.ErrContainerTimeout
		}
		return err
	}

	if exitCode != 0 {
		return customErr.ErrContainerFailed
	}

	return nil
}

func SanitizeContainerName(raw string) string {
	cleaned := containerNameRegex.ReplaceAllString(raw, "-")
	if cleaned == "" {
		cleaned = "untitled"
	}
	return "submission-" + cleaned
}

func buildCompileCommand(cfg CommandConfig) []string {
	switch cfg.LanguageType {
	case languages.CPP:
		versionFlag, _ := languages.GetVersionFlag(languages.CPP, cfg.LanguageVersion)
		return []string{
			"g++",
			"-o",
			filepath.Base(cfg.DirConfig.UserExecFilePath),
			"-std=" + versionFlag,
			filepath.Base(cfg.SourceFilePath),
		}
	default:
		return []string{}
	}
}

func buildEnvironmentVariables(cfg CommandConfig) ([]string, error) {
	timeEnv := make([]string, len(cfg.TestCases))
	memEnv := make([]string, len(cfg.TestCases))
	for i, tc := range cfg.TestCases {
		timeEnv[i] = strconv.FormatInt(tc.TimeLimitMs, 10)
		memEnv[i] = strconv.FormatInt(tc.MemoryLimitKB, 10)
	}

	// Build run command
	bin := filepath.Base(cfg.DirConfig.UserExecFilePath) // e.g. "solution" or "solution.py"
	runCmd, err := cfg.LanguageType.GetRunCommand(bin)
	if err != nil {
		return nil, err
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

	envVars := []string{
		"RUN_CMD=" + utils.ShellQuoteSlice(runCmd),
		"TIME_LIMITS_MS=" + utils.ShellQuoteSlice(timeEnv),
		"MEM_LIMITS_KB=" + utils.ShellQuoteSlice(memEnv),
		"INPUT_FILES=" + utils.ShellQuoteSlice(inputFilePaths),
		"USER_OUTPUT_FILES=" + utils.ShellQuoteSlice(userOutputFilePaths),
		"USER_ERROR_FILES=" + utils.ShellQuoteSlice(userErrorFilePaths),
		"USER_EXEC_RESULT_FILES=" + utils.ShellQuoteSlice(userExecResultFilePaths),
	}

	if cfg.RequiresCompiling {
		compileCmd := buildCompileCommand(cfg)
		envVars = append(envVars,
			"REQUIRES_COMPILATION=true",
			"COMPILE_CMD="+utils.ShellQuoteSlice(compileCmd),
			"SOURCE_FILE="+filepath.Base(cfg.SourceFilePath),
			"EXEC_FILE="+filepath.Base(cfg.DirConfig.UserExecFilePath),
			"COMPILE_ERR_FILE="+filepath.Base(cfg.DirConfig.CompileErrFilePath),
		)
	}

	return envVars, nil
}

func buildContainerConfig(
	userPackageDirPath string,
	dockerImage string,
	env []string,
) *container.Config {
	stopTimeout := int(2)

	return &container.Config{
		Image:       dockerImage,
		Cmd:         []string{"bash", "-lc", constants.DockerTestScript},
		WorkingDir:  userPackageDirPath,
		Env:         env,
		User:        "runner",
		StopTimeout: &stopTimeout,
		StopSignal:  "SIGKILL",
	}
}

func buildHostConfig(testCases []messages.TestCase) *container.HostConfig {
	maxKB := constants.MinContainerMemoryKB
	for _, tc := range testCases {
		if tc.MemoryLimitKB > maxKB {
			maxKB = tc.MemoryLimitKB
		}
	}

	// Headroom: 20% + at least 64MB for runtime/script overhead
	headroomKB := maxKB / 5
	minHeadroomKB := int64(64 * 1024)
	if headroomKB < minHeadroomKB {
		headroomKB = minHeadroomKB
	}

	containerKB := maxKB + headroomKB
	containerBytes := containerKB * 1024

	return &container.HostConfig{
		AutoRemove:  false,
		NetworkMode: container.NetworkMode("none"),
		Resources: container.Resources{
			Memory:     containerBytes,
			MemorySwap: containerBytes,
			PidsLimit:  func(v int64) *int64 { return &v }(8),
			CPUPeriod:  100_000,
			CPUQuota:   100_000,
		},
		SecurityOpt:  []string{"no-new-privileges"},
		CgroupnsMode: container.CgroupnsModePrivate,
		IpcMode:      container.IpcMode("private"),
		CapDrop:      []string{"ALL"},
	}
}

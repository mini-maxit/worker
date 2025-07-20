package services

import (
	"fmt"
	"os"

	"github.com/mini-maxit/worker/compiler"
	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/utils"

	s "github.com/mini-maxit/worker/internal/solution"
	"go.uber.org/zap"
)

type RunnerService interface {
	RunSolution(task *TaskForRunner, messageID string) s.Result
}

type runnerService struct {
	logger          *zap.SugaredLogger
	executor        executor.Executor
	testCaseService TestCaseService
	solutionService SolutionService
}

func NewRunnerService(
	executor executor.Executor,
	testCaseService TestCaseService,
	solutionService SolutionService,
) RunnerService {
	logger := logger.NewNamedLogger("runnerService")
	return &runnerService{
		executor:        executor,
		testCaseService: testCaseService,
		solutionService: solutionService,
		logger:          logger,
	}
}

func (r *runnerService) RunSolution(task *TaskForRunner, messageID string) s.Result {
	r.logger.Infof("Initializing solutionCompiler [MsgID: %s]", messageID)
	// Init appropriate solutionCompiler
	solutionCompiler, err := compiler.Initialize(task.languageType, task.languageVersion, messageID)
	if err != nil {
		r.logger.Errorf("Error initializing solutionCompiler [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			StatusCode: s.InitializationError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Creating user output directory [MsgID: %s]", messageID)

	err = os.Mkdir(fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.userOutputDirName), os.ModePerm)
	if err != nil {
		r.logger.Errorf("Error creating user output directory [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Created user output directory [MsgID: %s]", messageID)

	filePath, err := r.solutionService.PrepareSolutionFilePath(
		task.taskFilesDirPath,
		task.solutionFileName,
		solutionCompiler,
		messageID)
	if err != nil {
		r.logger.Errorf("Error preparing solution file path [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.CompilationError,
			Message:    err.Error(),
		}
	}

	err = r.solutionService.SetupOutputErrorFiles(task.taskFilesDirPath, task.userOutputDirName, len(task.timeLimits))
	if err != nil {
		r.logger.Errorf("Error setting up IO files [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}
	r.logger.Infof("Created output and error files [MsgID: %s]", messageID)

	r.logger.Infof("Running and evaluating test cases [MsgID: %s]", messageID)
	solutionResult := r.runAndEvaluateTestCases(task, filePath, messageID)
	return solutionResult
}

func (r *runnerService) runAndEvaluateTestCases(
	task *TaskForRunner,
	filePath, messageID string,
) s.Result {
	inputPath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.inputDirName)
	r.logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := utils.ParseInputFiles(inputPath)
	if err != nil {
		r.logger.Errorf("Error parsing input files [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}
	if len(inputFiles) != len(task.timeLimits) || len(inputFiles) != len(task.memoryLimits) {
		r.logger.Errorf("Mismatch in number of input files and limits [MsgID: %s]", messageID)
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    errors.ErrInputOutputMismatch.Error(),
		}
	}

	dockerImage, err := task.languageType.GetDockerImage(task.languageVersion)
	if err != nil {
		r.logger.Errorf("Error getting Docker image [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Running solution in Docker image %s [MsgID: %s]", dockerImage, messageID)
	err = r.executor.ExecuteCommand(filePath, messageID, executor.CommandConfig{
		WorkspaceDir:       task.taskFilesDirPath,
		InputDirName:       task.inputDirName,
		OutputDirName:      task.userOutputDirName,
		ExecResultFileName: constants.ExecResultFileName,
		DockerImage:        dockerImage,
		TimeLimits:         task.timeLimits,
		MemoryLimits:       task.memoryLimits,
	})
	if err != nil {
		r.logger.Errorf("Error executing command [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}
	r.logger.Infof("Execution completed [MsgID: %s]", messageID)

	return r.testCaseService.EvaluateAllTestCases(task, messageID, inputFiles)
}

package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/logger"
	s "github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/verifier"
	"go.uber.org/zap"
)

type RunnerService interface {
	RunSolution(task *TaskForRunner, messageID string) s.Result
	parseInputFiles(inputDir string) ([]string, error)
}

type runnerService struct {
	logger *zap.SugaredLogger
}

func NewRunnerService() RunnerService {
	logger := logger.NewNamedLogger("runnerService")

	return &runnerService{
		logger: logger,
	}
}

func (r *runnerService) RunSolution(task *TaskForRunner, messageID string) s.Result {
	r.logger.Infof("Initializing solutionExecutor [MsgID: %s]", messageID)
	// Init appropriate solutionExecutor
	solutionExecutor, err := initializeSolutionExecutor(task.languageType, task.languageVersion, messageID)
	if err != nil {
		r.logger.Errorf("Error initializing solutionExecutor [MsgID: %s]: %s", messageID, err.Error())
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

	r.logger.Infof("Creaed user output directory [MsgID: %s]", messageID)

	filePath, err := r.prepareSolutionFilePath(task, solutionExecutor, messageID)
	if err != nil {
		r.logger.Errorf("Error preparing solution file path [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.CompilationError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Running and evaluating test cases [MsgID: %s]", messageID)
	solutionResult := r.runAndEvaluateTestCases(solutionExecutor, task, filePath, messageID)
	return solutionResult
}

func (r *runnerService) prepareSolutionFilePath(
	task *TaskForRunner,
	solutionExecutor executor.Executor,
	messageID string,
) (string, error) {
	var filePath string
	var err error
	solutionFilePath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.solutionFileName)
	if solutionExecutor.RequiresCompilation() {
		r.logger.Infof("Compiling solution file %s [MsgID: %s]", solutionFilePath, messageID)
		filePath, err = solutionExecutor.Compile(solutionFilePath, task.taskFilesDirPath, messageID)
		if err != nil {
			r.logger.Errorf("Error compiling solution file %s [MsgID: %s]: %s", solutionFilePath, messageID, err.Error())
			return "", err
		}
	} else {
		filePath = solutionFilePath
	}

	return filePath, nil
}

func (r *runnerService) runAndEvaluateTestCases(
	solutionExecutor executor.Executor,
	task *TaskForRunner,
	filePath, messageID string,
) s.Result {
	inputPath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.inputDirName)
	r.logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	if len(inputFiles) != len(task.timeLimits) || len(inputFiles) != len(task.memoryLimits) {
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    constants.SolutionMessageLimitsMismatch,
		}
	}

	verifier := verifier.NewDefaultVerifier([]string{"-w"})
	testCases := make([]s.TestResult, len(inputFiles))
	solutionStatus := s.Success
	solutionMessages := make([]string, len(inputFiles))
	for i, inputPath := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, task.userOutputDirName, (i + 1))
		stderrPath := fmt.Sprintf("%s/%s/%d.err", task.taskFilesDirPath, task.userOutputDirName, (i + 1))
		commandConfig := executor.CommandConfig{
			StdinPath:     inputPath,
			StdoutPath:    outputPath,
			StderrPath:    stderrPath,
			TimeLimit:     task.timeLimits[i],
			MemoryLimit:   task.memoryLimits[i],
			ChrootDirPath: task.chrootDirPath,
			UseChroot:     task.useChroot,
		}

		taskForEvaluation := &TaskForEvaluation{
			taskFilesDirPath: task.taskFilesDirPath,
			outputDirName:    task.outputDirName,
			timeLimit:        task.timeLimits[i],
			memoryLimit:      task.memoryLimits[i],
		}

		execResult := solutionExecutor.ExecuteCommand(filePath, messageID, commandConfig)
		testCases[i], solutionStatus, solutionMessages[i] = r.evaluateTestCase(
			execResult,
			verifier,
			taskForEvaluation,
			outputPath,
			stderrPath,
			(i + 1),
		)
	}

	// Construct final message summarizing the results
	var finalMessage string
	if solutionStatus == s.Success {
		finalMessage = constants.SolutionMessageSuccess
	} else {
		for i, message := range solutionMessages {
			if i != 0 {
				finalMessage += ", "
			}
			finalMessage += fmt.Sprintf("%d. %s", (i + 1), message)
		}
		finalMessage += "."
	}

	return s.Result{
		OutputDir:   task.userOutputDirName,
		StatusCode:  solutionStatus,
		Message:     finalMessage,
		TestResults: testCases,
	}
}

func (r *runnerService) evaluateTestCase(
	execResult *executor.ExecutionResult,
	verifier verifier.Verifier,
	task *TaskForEvaluation,
	outputPath, stderrPath string,
	testCase int,
) (s.TestResult, s.ResultStatus, string) {
	solutionStatus := s.Success
	solutionMessage := constants.SolutionMessageSuccess

	switch execResult.ExitCode {
	case constants.ExitCodeSuccess:
		expectedFilePath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, task.outputDirName, testCase)

		passed, err := verifier.CompareOutput(outputPath, expectedFilePath, stderrPath)
		if err != nil {
			return s.TestResult{
				Passed:        false,
				ExecutionTime: execResult.ExecTime,
				StatusCode:    s.TestCaseStatus(s.InternalError),
				ErrorMessage:  err.Error(),
				Order:         testCase,
			}, s.InternalError, constants.SolutionMessageInternalError
		}

		statusCode := s.TestCasePassed

		if !passed {
			solutionStatus = s.TestFailed
			solutionMessage = constants.SolutionMessageOutputDifference
			statusCode = s.OutputDifference
		}

		return s.TestResult{
			Passed:        passed,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    statusCode,
			ErrorMessage:  "",
			Order:         testCase,
		}, solutionStatus, solutionMessage

	case constants.ExitCodeTimeLimitExceeded:
		message := fmt.Sprintf("Solution timed out after %d s", task.timeLimit)
		return s.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    s.TimeLimitExceeded,
			ErrorMessage:  message,
			Order:         testCase,
		}, s.TestFailed, constants.SolutionMessageTimeout

	case constants.ExitCodeMemoryLimitExceeded:
		message := fmt.Sprintf("Solution exceeded memory limit of %d MB", task.memoryLimit)
		return s.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    s.MemoryLimitExceeded,
			ErrorMessage:  message,
			Order:         testCase,
		}, s.TestFailed, constants.SolutionMessageMemoryLimitExceeded

	default:
		return s.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    s.RuntimeError,
			ErrorMessage:  execResult.Message,
			Order:         testCase,
		}, s.TestFailed, constants.SolutionMessageRuntimeError
	}
}

func initializeSolutionExecutor(
	languageType languages.LanguageType,
	languageVersion,
	messageID string,
) (executor.Executor, error) {
	switch languageType {
	case languages.CPP:
		return executor.NewCppExecutor(languageVersion, messageID)
	default:
		return nil, errors.ErrInvalidLanguageType
	}
}

func (r *runnerService) parseInputFiles(inputDir string) ([]string, error) {
	dirEntries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".in") {
			result = append(result, fmt.Sprintf("%s/%s", inputDir, entry.Name()))
		}
	}
	if len(result) == 0 {
		return nil, errors.ErrEmptyInputDirectory
	}

	return result, nil
}

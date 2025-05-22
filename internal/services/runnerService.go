package services

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mini-maxit/worker/compiler"
	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/constants"
	myErrors "github.com/mini-maxit/worker/internal/errors"
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
	logger   *zap.SugaredLogger
	executor *executor.DockerExecutor
}

func NewRunnerService() (RunnerService, error) {
	logger := logger.NewNamedLogger("runnerService")
	executor, err := executor.NewDockerExecutor()
	if err != nil {
		logger.Errorf("Error creating Docker executor: %s", err.Error())
		return nil, err
	}
	return &runnerService{
		logger:   logger,
		executor: executor,
	}, nil
}

func (r *runnerService) RunSolution(task *TaskForRunner, messageID string) s.Result {
	r.logger.Infof("Initializing solutionCompiler [MsgID: %s]", messageID)
	// Init appropriate solutionCompiler
	solutionCompiler, err := initializeSolutionCompiler(task.languageType, task.languageVersion, messageID)
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

	r.logger.Infof("Creaed user output directory [MsgID: %s]", messageID)

	filePath, err := r.prepareSolutionFilePath(task, solutionCompiler, messageID)
	if err != nil {
		r.logger.Errorf("Error preparing solution file path [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.CompilationError,
			Message:    err.Error(),
		}
	}

	err = r.setupOutputErrorFiles(task.taskFilesDirPath, task.userOutputDirName, len(task.timeLimits))
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

func (r *runnerService) prepareSolutionFilePath(
	task *TaskForRunner,
	solutionCompiler compiler.Compiler,
	messageID string,
) (string, error) {
	var filePath string
	var err error
	solutionFilePath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.solutionFileName)
	if solutionCompiler.RequiresCompilation() {
		r.logger.Infof("Compiling solution file %s [MsgID: %s]", solutionFilePath, messageID)
		filePath, err = solutionCompiler.Compile(solutionFilePath, task.taskFilesDirPath, messageID)
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
	task *TaskForRunner,
	filePath, messageID string,
) s.Result {
	inputFiles, inputErrResult, err := r.prepareAndValidateInputFiles(task, messageID)
	if err != nil {
		return *inputErrResult
	}

	err = r.runSolutionInDocker(task, filePath, messageID)
	if err != nil {
		r.logger.Errorf("Error executing command [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}
	r.logger.Infof("Execution completed [MsgID: %s]", messageID)

	return r.evaluateAllTestCases(task, messageID, inputFiles)
}

// Helper: prepares and validates input files, returns ([]string, *s.Result, error).
func (r *runnerService) prepareAndValidateInputFiles(
	task *TaskForRunner,
	messageID string,
) ([]string, *s.Result, error) {
	inputPath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.inputDirName)
	r.logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		return nil, &s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}, err
	}
	if len(inputFiles) != len(task.timeLimits) || len(inputFiles) != len(task.memoryLimits) {
		return nil, &s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    constants.SolutionMessageLimitsMismatch,
		}, errors.New(constants.SolutionMessageLimitsMismatch)
	}
	return inputFiles, nil, nil
}

// Helper: runs the solution in Docker.
func (r *runnerService) runSolutionInDocker(task *TaskForRunner, filePath, messageID string) error {
	dockerImage, err := task.languageType.GetDockerImage(task.languageVersion)
	if err != nil {
		r.logger.Errorf("Error getting Docker image [MsgID: %s]: %s", messageID, err.Error())
		return err
	}

	r.logger.Infof("Running solution in Docker image %s [MsgID: %s]", dockerImage, messageID)
	return r.executor.ExecuteCommand(filePath, messageID, executor.CommandConfig{
		WorkspaceDir:       task.taskFilesDirPath,
		InputDirName:       task.inputDirName,
		OutputDirName:      task.userOutputDirName,
		ExecResultFileName: constants.ExecResultFileName,
		DockerImage:        dockerImage,
		TimeLimits:         task.timeLimits,
		MemoryLimits:       task.memoryLimits,
	})
}

// Helper: evaluates all test cases and constructs the result.
func (r *runnerService) evaluateAllTestCases(task *TaskForRunner, messageID string, inputFiles []string) s.Result {
	verifier := verifier.NewDefaultVerifier([]string{"-w"})
	testCases := make([]s.TestResult, len(inputFiles))
	solutionStatuses := make([]s.ResultStatus, len(inputFiles))
	solutionMessages := make([]string, len(inputFiles))

	execResultsFilePath := fmt.Sprintf("%s/%s/%s",
		task.taskFilesDirPath,
		task.userOutputDirName,
		constants.ExecResultFileName)
	execResults, err := r.ReadExecutionResultFile(execResultsFilePath, len(inputFiles))
	if err != nil {
		r.logger.Errorf("Error reading execution result file [MsgID: %s]: %s", messageID, err.Error())
		return s.Result{
			OutputDir:  task.userOutputDirName,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	for i := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, task.userOutputDirName, (i + 1))
		stderrPath := fmt.Sprintf("%s/%s/%d.err", task.taskFilesDirPath, task.userOutputDirName, (i + 1))

		taskForEvaluation := &TaskForEvaluation{
			taskFilesDirPath: task.taskFilesDirPath,
			outputDirName:    task.outputDirName,
			timeLimit:        task.timeLimits[i],
			memoryLimit:      task.memoryLimits[i],
		}

		testCases[i], solutionStatuses[i], solutionMessages[i] = r.evaluateTestCase(
			execResults[i],
			verifier,
			taskForEvaluation,
			outputPath,
			stderrPath,
			(i + 1),
		)
	}

	// Construct final message summarizing the results
	var finalMessage string
	finalStatus := s.Success
	for _, status := range solutionStatuses {
		if status != s.Success {
			finalStatus = s.TestFailed
			break
		}
	}

	if finalStatus == s.Success {
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
		StatusCode:  finalStatus,
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
		message := fmt.Sprintf(
			constants.TestCaseMessageTimeOut,
			task.timeLimit,
		)
		return s.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    s.TimeLimitExceeded,
			ErrorMessage:  message,
			Order:         testCase,
		}, s.TestFailed, constants.SolutionMessageTimeout

	case constants.ExitCodeMemoryLimitExceeded:
		message := fmt.Sprintf(
			constants.TestCaseMessageMemoryLimitExceeded,
			task.memoryLimit,
		)
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
			ErrorMessage:  constants.TestCaseMessageRuntimeError,
			Order:         testCase,
		}, s.TestFailed, constants.SolutionMessageRuntimeError
	}
}

func initializeSolutionCompiler(
	languageType languages.LanguageType,
	languageVersion,
	messageID string,
) (compiler.Compiler, error) {
	switch languageType {
	case languages.CPP:
		return compiler.NewCppExecutor(languageVersion, messageID)
	default:
		return nil, myErrors.ErrInvalidLanguageType
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
		return nil, myErrors.ErrEmptyInputDirectory
	}

	return result, nil
}

func (r *runnerService) setupOutputErrorFiles(taskDirPath, outputDirName string, numberOfFiles int) error {
	for i := 1; i <= numberOfFiles; i++ {
		outputFilePath := fmt.Sprintf("%s/%s/%d.out", taskDirPath, outputDirName, i)
		_, err := os.Create(outputFilePath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}

		errorFilePath := fmt.Sprintf("%s/%s/%d.err", taskDirPath, outputDirName, i)
		_, err = os.Create(errorFilePath)
		if err != nil {
			return fmt.Errorf("failed to create error file: %w", err)
		}
	}
	return nil
}

func (r *runnerService) ReadExecutionResultFile(
	filePath string,
	numberOfTest int,
) ([]*executor.ExecutionResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	results := make([]*executor.ExecutionResult, numberOfTest)
	for i := range results {
		var exitCode int
		var execTime float64
		_, err := fmt.Fscanf(file, "%d %f\n", &exitCode, &execTime)
		if err != nil {
			r.logger.Errorf("Failed to read line %d: %s", i, err)
			return nil, err
		}
		results[i] = &executor.ExecutionResult{
			ExitCode: exitCode,
			ExecTime: execTime,
		}
	}
	return results, nil
}

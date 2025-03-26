package services

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/logger"
	s "github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/verifier"
	"go.uber.org/zap"
)

type RunnerService interface {
	RunSolution(task *TaskForRunner, messageID string) s.SolutionResult
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

func (r *runnerService) RunSolution(task *TaskForRunner, messageID string) s.SolutionResult {
	r.logger.Infof("Initializing executor [MsgID: %s]", messageID)
	// Init appropriate executor
	var exec executor.Executor
	var err error
	switch task.languageType {
	case languages.CPP:
		r.logger.Infof("Initializing C++ executor [MsgID: %s]", messageID)
		exec, err = executor.NewCppExecutor(task.languageVersion, messageID)
	default:
		r.logger.Errorf("Invalid language type supplied [MsgID: %s]", messageID)
		return s.SolutionResult{
			Success: false,
			Message: constants.SolutionMessageInvalidLanguageType,
		}
	}
	if err != nil {
		r.logger.Errorf("Error occured during initialization [MsgID: %s]", messageID)
		return s.SolutionResult{
			Success:    false,
			StatusCode: s.InitializationError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Creating user output directory [MsgID: %s]", messageID)

	userOutputDir := "user-output"
	err = os.Mkdir(fmt.Sprintf("%s/%s", task.taskFilesDirPath, userOutputDir), os.ModePerm)
	if err != nil {
		r.logger.Errorf("Error creating user output directory [MsgID: %s]: %s", messageID, err.Error())
		return s.SolutionResult{
			Success:    false,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Creaed user output directory [MsgID: %s]", messageID)

	var filePath string
	if exec.IsCompiled() {
		solutionFilePath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.solutionFileName)
		r.logger.Infof("Compiling solution file %s [MsgID: %s]", solutionFilePath, messageID)
		filePath, err = exec.Compile(solutionFilePath, task.taskFilesDirPath, messageID)
		if err != nil {
			r.logger.Errorf("Error compiling solution file %s [MsgID: %s]: %s", solutionFilePath, messageID, err.Error())
			return s.SolutionResult{
				OutputDir:  userOutputDir,
				Success:    false,
				StatusCode: s.CompilationError,
				Message:    err.Error(),
			}
		}
	} else {
		filePath = task.solutionFileName
	}

	inputPath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.inputDirName)
	r.logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		r.logger.Errorf("Error reading input files from %s [MsgID: %s]: %s", inputPath, messageID, err.Error())
		return s.SolutionResult{
			OutputDir:  userOutputDir,
			Success:    false,
			StatusCode: s.Failed,
			Message:    err.Error(),
		}
	}

	if len(inputFiles) != len(task.timeLimits) || len(inputFiles) != len(task.memoryLimits) {
		r.logger.Errorf("Invalid number of limits, got %d input files, %d time limits and %d memory limits [MsgID: %s]", len(inputFiles), len(task.timeLimits), len(task.memoryLimits), messageID)
		return s.SolutionResult{
			OutputDir:  userOutputDir,
			Success:    false,
			StatusCode: s.Failed,
			Message:    constants.SolutionMessageLimitsMismatch,
		}
	}

	r.logger.Infof("Executing solution [MsgID: %s]", messageID)
	verifier := verifier.NewDefaultVerifier()
	testCases := make([]s.TestResult, len(inputFiles))
	solutionSuccess := true
	solutionStatus := s.Success
	solutionMessage := constants.SolutionMessageSuccess
	for i, inputPath := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, userOutputDir, (i + 1))
		stderrPath := fmt.Sprintf("%s/%s/%d.err", task.taskFilesDirPath, userOutputDir, (i + 1))
		commandConfig := executor.CommandConfig{
			StdinPath:     inputPath,
			StdoutPath:    outputPath,
			StderrPath:    stderrPath,
			TimeLimit:     task.timeLimits[i],
			MemoryLimit:   task.memoryLimits[i],
			ChrootDirPath: task.chrootDirPath,
			UseChroot:     task.useChroot,
		}

		execTimeStart := time.Now()
		execResult := exec.ExecuteCommand(filePath, messageID, commandConfig)
		execTimeEnd := time.Now()

		execTime := execTimeEnd.Sub(execTimeStart).Seconds()
		switch execResult.ExitCode {
		case constants.ExitCodeSuccess:
			r.logger.Infof("Comparing output %s with expected output [MsgID: %s]", outputPath, messageID)
			expectedFilePath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, task.outputDirName, (i + 1))
			result, difference, err := verifier.CompareOutput(outputPath, expectedFilePath)
			if err != nil {
				r.logger.Errorf("Error comparing output %s with expected output [MsgID: %s]: %s", outputPath, messageID, err.Error())
				solutionSuccess = false
				difference = err.Error()
			}
			if !result {
				solutionSuccess = false
			}
			err = StoreTestCaseDifferenceInErrFile((i + 1), difference, commandConfig.StderrPath)
			if err != nil {
				r.logger.Errorf("Error storing difference in error file [MsgID: %s]: %s", messageID, err.Error())
				solutionSuccess = false
				difference = err.Error()
			}
			testCases[i] = s.TestResult{
				Passed:        result,
				ExecutionTime: execTime,
				ErrorMessage:  difference,
				Order:         (i + 1),
			}
		case constants.ExitCodeTimeLimitExceeded:
			r.logger.Errorf("Time limit exceeded while executing solution [MsgID: %s]", messageID)
			testCases[i] = s.TestResult{
				Passed:        false,
				ExecutionTime: execTime,
				ErrorMessage:  constants.TestMessageTimeLimitExceeded,
				Order:         (i + 1),
			}
			solutionSuccess = false
			solutionStatus = s.RuntimeError
			solutionMessage = constants.SolutionMessageTimeout
		case constants.ExitCodeMemoryLimitExceeded:
			r.logger.Errorf("Memory limit exceeded while executing solution [MsgID: %s]", messageID)
			testCases[i] = s.TestResult{
				Passed:        false,
				ExecutionTime: execTime,
				ErrorMessage:  constants.TestMessageMemoryLimitExceeded,
				Order:         (i + 1),
			}
			solutionSuccess = false
			solutionStatus = s.RuntimeError
			solutionMessage = constants.SolutionMessageMemoryLimitExceeded
		default:
			r.logger.Errorf("Error executing solution [MsgID: %s]: %s", messageID, execResult.Message)
			testCases[i] = s.TestResult{
				Passed:        false,
				ExecutionTime: execTime,
				ErrorMessage:  execResult.Message,
				Order:         (i + 1),
			}
			solutionSuccess = false
			solutionStatus = s.RuntimeError
			solutionMessage = constants.SolutionMessageRuntimeError
		}
	}

	r.logger.Infof("Solution executed successfully [MsgID: %s]", messageID)
	return s.SolutionResult{
		OutputDir:   userOutputDir,
		Success:     solutionSuccess,
		StatusCode:  solutionStatus,
		Message:     solutionMessage,
		TestResults: testCases,
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
		return nil, fmt.Errorf("empty input directory, verify task files")
	}

	return result, nil
}

func StoreTestCaseDifferenceInErrFile(order int, difference string, errFilePath string) error {
	if difference == "" {
		return nil
	}
	errFile, err := os.Create(errFilePath)
	if err != nil {
		return err
	}
	defer errFile.Close()

	_, err = errFile.WriteString(fmt.Sprintf("Test case %d:\n", order))
	if err != nil {
		return err
	}
	_, err = errFile.WriteString(difference)
	if err != nil {
		return err
	}

	return nil
}

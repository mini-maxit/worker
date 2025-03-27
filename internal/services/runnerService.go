package services

import (
	"fmt"
	"os"
	"strings"

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
	r.logger.Infof("Initializing solutionExecutor [MsgID: %s]", messageID)
	// Init appropriate solutionExecutor
	var solutionExecutor executor.Executor
	var err error
	switch task.languageType {
	case languages.CPP:
		r.logger.Infof("Initializing C++ solutionExecutor [MsgID: %s]", messageID)
		solutionExecutor, err = executor.NewCppExecutor(task.languageVersion, messageID)
	case languages.Python:
		r.logger.Infof("Initializing Python solutionExecutor [MsgID: %s]", messageID)
		solutionExecutor, err = executor.NewPythonExecutor(task.languageVersion, messageID)
	default:
		r.logger.Errorf("Invalid language type supplied [MsgID: %s]", messageID)
		return s.SolutionResult{
			StatusCode: s.InitializationError,
			Message:    constants.SolutionMessageInvalidLanguageType,
		}
	}
	if err != nil {
		r.logger.Errorf("Error occured during initialization [MsgID: %s]", messageID)
		return s.SolutionResult{
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
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Creaed user output directory [MsgID: %s]", messageID)

	var filePath string
	solutionFilePath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.solutionFileName)
	if solutionExecutor.RequiresCompilation() {
		r.logger.Infof("Compiling solution file %s [MsgID: %s]", solutionFilePath, messageID)
		filePath, err = solutionExecutor.Compile(solutionFilePath, task.taskFilesDirPath, messageID)
		if err != nil {
			r.logger.Errorf("Error compiling solution file %s [MsgID: %s]: %s", solutionFilePath, messageID, err.Error())
			return s.SolutionResult{
				OutputDir:  userOutputDir,
				StatusCode: s.CompilationError,
				Message:    err.Error(),
			}
		}
	} else {
		filePath = solutionFilePath
	}

	inputPath := fmt.Sprintf("%s/%s", task.taskFilesDirPath, task.inputDirName)
	r.logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		r.logger.Errorf("Error reading input files from %s [MsgID: %s]: %s", inputPath, messageID, err.Error())
		return s.SolutionResult{
			OutputDir:  userOutputDir,
			StatusCode: s.InternalError,
			Message:    err.Error(),
		}
	}

	if len(inputFiles) != len(task.timeLimits) || len(inputFiles) != len(task.memoryLimits) {
		r.logger.Errorf("Invalid number of limits, got %d input files, %d time limits and %d memory limits [MsgID: %s]", len(inputFiles), len(task.timeLimits), len(task.memoryLimits), messageID)
		return s.SolutionResult{
			OutputDir:  userOutputDir,
			StatusCode: s.InternalError,
			Message:    constants.SolutionMessageLimitsMismatch,
		}
	}

	r.logger.Infof("Executing solution [MsgID: %s]", messageID)
	verifier := verifier.NewDefaultVerifier("-w")
	testCases := make([]s.TestResult, len(inputFiles))
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

		execResult := solutionExecutor.ExecuteCommand(filePath, messageID, commandConfig)
		switch execResult.ExitCode {
		case constants.ExitCodeSuccess:
			r.logger.Infof("Comparing output %s with expected output [MsgID: %s]", outputPath, messageID)
			expectedFilePath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, task.outputDirName, (i + 1))

			passed, err := verifier.CompareOutput(outputPath, expectedFilePath, stderrPath)
			if err != nil {
				r.logger.Errorf("Error comparing output %s with expected output %s [MsgID: %s]: %s", outputPath, expectedFilePath, messageID, err.Error())
				testCases[i] = s.TestResult{
					Passed:       false,
					ErrorMessage: err.Error(),
					Order:        (i + 1),
				}
				solutionStatus = s.InternalError
				solutionMessage = constants.SolutionMessageInternalError
				continue
			}

			if !passed {
				solutionStatus = s.TestFailed
				solutionMessage = constants.SolutionMessageTestFailed
			}

			testCases[i] = s.TestResult{
				Passed: passed,
				Order:  (i + 1),
			}
		case constants.ExitCodeTimeLimitExceeded:
			r.logger.Errorf("Time limit exceeded while executing solution [MsgID: %s]", messageID)
			testCases[i] = s.TestResult{
				Passed:       false,
				ErrorMessage: constants.TestMessageTimeLimitExceeded,
				Order:        (i + 1),
			}
			solutionStatus = s.TimeLimitExceeded
			solutionMessage = constants.SolutionMessageTimeout
		case constants.ExitCodeMemoryLimitExceeded:
			r.logger.Errorf("Memory limit exceeded while executing solution [MsgID: %s]", messageID)
			testCases[i] = s.TestResult{
				Passed:       false,
				ErrorMessage: constants.TestMessageMemoryLimitExceeded,
				Order:        (i + 1),
			}
			solutionStatus = s.MemoryLimitExceeded
			solutionMessage = constants.SolutionMessageMemoryLimitExceeded
		default:
			r.logger.Errorf("Error executing solution [MsgID: %s]: %s", messageID, execResult.Message)
			testCases[i] = s.TestResult{
				Passed:       false,
				ErrorMessage: execResult.Message,
				Order:        (i + 1),
			}
			solutionStatus = s.RuntimeError
			solutionMessage = constants.SolutionMessageRuntimeError
		}
	}

	r.logger.Infof("Solution executed successfully [MsgID: %s]", messageID)
	return s.SolutionResult{
		OutputDir:   userOutputDir,
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

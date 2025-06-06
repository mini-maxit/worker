package services

import (
	"fmt"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
	s "github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
	"github.com/mini-maxit/worker/verifier"
	"go.uber.org/zap"
)

type TestCaseService interface {
	EvaluateAllTestCases(task *TaskForRunner, messageID string, inputFiles []string) s.Result
}

type testCaseService struct {
	logger *zap.SugaredLogger
}

func NewTestCaseService() TestCaseService {
	logger := logger.NewNamedLogger("TestCaseService")
	return &testCaseService{
		logger: logger,
	}
}

func (t *testCaseService) EvaluateAllTestCases(task *TaskForRunner, messageID string, inputFiles []string) s.Result {
	verifier := verifier.NewDefaultVerifier([]string{"-w"})
	testCases := make([]s.TestResult, len(inputFiles))
	solutionStatuses := make([]s.ResultStatus, len(inputFiles))
	solutionMessages := make([]string, len(inputFiles))

	execResultsFilePath := fmt.Sprintf("%s/%s/%s",
		task.taskFilesDirPath,
		task.userOutputDirName,
		constants.ExecResultFileName)
	execResults, err := utils.ReadExecutionResultFile(execResultsFilePath, len(inputFiles))
	if err != nil {
		t.logger.Errorf("Error reading execution result file [MsgID: %s]: %s", messageID, err.Error())
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

		testCases[i], solutionStatuses[i], solutionMessages[i] = t.evaluateTestCase(
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

func (t *testCaseService) evaluateTestCase(
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

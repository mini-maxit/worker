package verifier

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/solution"
	"go.uber.org/zap"
)

// Returns true if the output and expected output are the same.
// Returns false otherwise.
// Returns an error if an error occurs.
type Verifier interface {
	EvaluateAllTestCases(dirConfig *packager.TaskDirConfig, messageID string, limits []solution.Limit) solution.Result
}

type verifier struct {
	flags  []string
	logger *zap.SugaredLogger
}

func NewVerifier(flags []string) Verifier {
	return &verifier{
		flags:  flags,
		logger: logger.NewNamedLogger("verifier"),
	}
}

// CompareOutput compares the output file with the expected output file.
// It returns true if the files are the same, false otherwise.
// It returns a string of differences if there are any.
// It returns an error if any issue occurs during the comparison.
func (v *verifier) compareOutput(outputPath, expectedFilePath, stderrPath string) (bool, error) {
	outputPath = filepath.Clean(outputPath)
	expectedFilePath = filepath.Clean(expectedFilePath)

	args := append([]string{}, v.flags...)

	args = append(args, outputPath, expectedFilePath)
	diffCmd := exec.Command("diff", args...)

	diffCmdOutput, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, fmt.Errorf("error opening output file %s: %w", outputPath, err)
	}
	defer diffCmdOutput.Close()

	diffCmd.Stdout = diffCmdOutput
	diffCmd.Stderr = diffCmdOutput
	err = diffCmd.Run()
	var diffExitCode int
	if diffCmd.ProcessState != nil {
		diffExitCode = diffCmd.ProcessState.ExitCode()
	}
	if err != nil {
		if diffCmd.ProcessState != nil && diffExitCode == constants.ExitCodeDifference {
			return false, nil
		}
		if diffCmd.ProcessState == nil {
			return false, fmt.Errorf("error running diff command: %w (process state unavailable)", err)
		}
		if rb, rerr := os.ReadFile(stderrPath); rerr == nil && len(rb) > 0 {
			return false, fmt.Errorf("error running diff command: %w: %s", err, string(rb))
		}
		return false, fmt.Errorf("error running diff command: %w", err)
	}

	return true, nil
}

func (v *verifier) EvaluateAllTestCases(
	dirConfig *packager.TaskDirConfig,
	messageID string,
	limits []solution.Limit,
) solution.Result {
	testResults := make([]solution.TestResult, len(limits))
	solutionStatuses := make([]solution.ResultStatus, len(limits))
	solutionMessages := make([]string, len(limits))
	numberOfTests := len(limits)

	execResults, err := v.readExecutionResultFiles(dirConfig.UserExecResultDirPath, numberOfTests)
	if err != nil {
		v.logger.Errorf("Error reading execution result file [MsgID: %s]: %s", messageID, err.Error())
		return solution.Result{
			StatusCode: solution.InternalError,
			Message:    err.Error(),
		}
	}

	for i := range numberOfTests {
		outputPath := fmt.Sprintf("%s/%d.out", dirConfig.UserOutputDirPath, (i + 1))
		// expected outputs are stored in the package outputs directory
		expectedOutputPath := fmt.Sprintf("%s/%d.out", dirConfig.OutputDirPath, (i + 1))
		stderrPath := fmt.Sprintf("%s/%d.err", dirConfig.UserErrorDirPath, (i + 1))

		testResults[i], solutionStatuses[i], solutionMessages[i] = v.evaluateTestCase(
			outputPath,
			expectedOutputPath,
			stderrPath,
			execResults[i],
			limits[i].TimeMs,
			limits[i].MemoryKb,
			(i + 1),
		)
	}

	// Construct final message summarizing the results

	finalMessage, finalStatus := v.constructFinalMessage(solutionStatuses, solutionMessages)

	return solution.Result{
		StatusCode:  finalStatus,
		Message:     finalMessage,
		TestResults: testResults,
	}
}

func (v *verifier) constructFinalMessage(
	statuses []solution.ResultStatus,
	messages []string,
) (string, solution.ResultStatus) {
	var finalMessage string
	finalStatus := solution.Success
	for _, status := range statuses {
		if status != solution.Success {
			finalStatus = solution.TestFailed
			break
		}
	}

	if finalStatus == solution.Success {
		finalMessage = constants.SolutionMessageSuccess
	} else {
		for i, message := range messages {
			if i != 0 {
				finalMessage += ", "
			}
			finalMessage += fmt.Sprintf("%d. %s", (i + 1), message)
		}
		finalMessage += "."
	}
	return finalMessage, finalStatus
}

func (v *verifier) readExecutionResultFiles(
	execResultDirPath string,
	numberOfTest int,
) ([]*executor.ExecutionResult, error) {
	results := make([]*executor.ExecutionResult, numberOfTest)
	for i := range results {
		filePath := filepath.Join(execResultDirPath, fmt.Sprintf("%d.res", i+1))
		file, err := os.Open(filePath)
		if err != nil {
			v.logger.Errorf("Failed to open execution result file %s: %s", filePath, err)
			return nil, err
		}
		defer file.Close()

		var exitCode int
		var execTime float64
		_, err = fmt.Fscanf(file, "%d %f\n", &exitCode, &execTime)
		if err != nil {
			v.logger.Errorf("Failed to read line %d: %s", i, err)
			return nil, err
		}
		results[i] = &executor.ExecutionResult{
			ExitCode: exitCode,
			ExecTime: execTime,
		}
	}
	return results, nil
}

func (v *verifier) evaluateTestCase(
	outputPath string,
	expectedOutputPath string,
	stderrPath string,
	execResult *executor.ExecutionResult,
	timeLimit int64,
	memoryLimit int64,
	testCaseIdx int,

) (solution.TestResult, solution.ResultStatus, string) {
	solutionStatus := solution.Success
	solutionMessage := constants.SolutionMessageSuccess

	switch execResult.ExitCode {
	case constants.ExitCodeSuccess:
		passed, err := v.compareOutput(outputPath, expectedOutputPath, stderrPath)
		if err != nil {
			return solution.TestResult{
				Passed:        false,
				ExecutionTime: execResult.ExecTime,
				StatusCode:    solution.TestCaseStatus(solution.InternalError),
				ErrorMessage:  err.Error(),
				Order:         testCaseIdx,
			}, solution.InternalError, constants.SolutionMessageInternalError
		}

		statusCode := solution.TestCasePassed

		if !passed {
			solutionStatus = solution.TestFailed
			solutionMessage = constants.SolutionMessageOutputDifference
			statusCode = solution.OutputDifference
		}

		return solution.TestResult{
			Passed:        passed,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    statusCode,
			ErrorMessage:  "",
			Order:         testCaseIdx,
		}, solutionStatus, solutionMessage

	case constants.ExitCodeTimeLimitExceeded:
		message := fmt.Sprintf(
			constants.TestCaseMessageTimeOut,
			timeLimit,
		)
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    solution.TimeLimitExceeded,
			ErrorMessage:  message,
			Order:         testCaseIdx,
		}, solution.TestFailed, constants.SolutionMessageTimeout

	case constants.ExitCodeMemoryLimitExceeded:
		message := fmt.Sprintf(
			constants.TestCaseMessageMemoryLimitExceeded,
			memoryLimit,
		)
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    solution.MemoryLimitExceeded,
			ErrorMessage:  message,
			Order:         testCaseIdx,
		}, solution.TestFailed, constants.SolutionMessageMemoryLimitExceeded

	default:
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    solution.RuntimeError,
			ErrorMessage:  constants.TestCaseMessageRuntimeError,
			Order:         testCaseIdx,
		}, solution.TestFailed, constants.SolutionMessageRuntimeError
	}
}

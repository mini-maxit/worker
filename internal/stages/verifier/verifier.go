package verifier

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	"go.uber.org/zap"
)

// Returns true if the output and expected output are the same.
// Returns false otherwise.
// Returns an error if an error occurs.
type Verifier interface {
	EvaluateAllTestCases(
		dirConfig *packager.TaskDirConfig,
		testCases []messages.TestCase,
		messageID string,
	) solution.Result
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
func (v *verifier) compareOutput(outputPath, expectedFilePath, diffPath, errorPath string) (bool, error) {
	outputPath = filepath.Clean(outputPath)
	expectedFilePath = filepath.Clean(expectedFilePath)

	args := append([]string{}, v.flags...)

	args = append(args, outputPath, expectedFilePath)
	ctx := context.Background() // TODO: use a timeout context
	diffCmd := exec.CommandContext(ctx, "diff", args...)

	diffCmdOutput, err := os.OpenFile(diffPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, fmt.Errorf("error opening output file %s: %w", outputPath, err)
	}
	defer diffCmdOutput.Close()

	diffCmdStdErr, err := os.OpenFile(errorPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, fmt.Errorf("error opening error file %s: %w", errorPath, err)
	}
	defer diffCmdStdErr.Close()

	diffCmd.Stdout = diffCmdOutput
	diffCmd.Stderr = diffCmdStdErr
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
		if rb, rerr := os.ReadFile(errorPath); rerr == nil && len(rb) > 0 {
			return false, fmt.Errorf("error running diff command: %w: %s", err, string(rb))
		}
		return false, fmt.Errorf("error running diff command: %w", err)
	}

	return true, nil
}

func (v *verifier) EvaluateAllTestCases(
	dirConfig *packager.TaskDirConfig,
	testCases []messages.TestCase,
	messageID string,
) solution.Result {
	v.logger.Infof("Starting evaluation of test cases [MsgID: %s]", messageID)
	numberOfTest := len(testCases)
	testResults := make([]solution.TestResult, numberOfTest)
	solutionStatuses := make([]solution.ResultStatus, numberOfTest)
	solutionMessages := make([]string, numberOfTest)

	execResults, err := v.readExecutionResultFiles(dirConfig.UserExecResultDirPath, numberOfTest)
	if err != nil {
		v.logger.Errorf("Error reading execution result file [MsgID: %s]: %s", messageID, err.Error())
		return solution.Result{
			StatusCode: solution.InternalError,
			Message:    err.Error(),
		}
	}

	for i, tc := range testCases {
		userOutputFile := filepath.Join(
			dirConfig.UserOutputDirPath,
			filepath.Base(tc.StdOutResult.Path))

		expectedOutputPath := filepath.Join(
			dirConfig.OutputDirPath,
			filepath.Base(tc.ExpectedOutput.Path))

		userDiffPath := filepath.Join(
			dirConfig.UserDiffDirPath,
			filepath.Base(tc.DiffResult.Path))

		userErrorPath := filepath.Join(
			dirConfig.UserErrorDirPath,
			filepath.Base(tc.StdErrResult.Path))

		testResults[i], solutionStatuses[i], solutionMessages[i] = v.evaluateTestCase(
			userOutputFile,
			expectedOutputPath,
			userDiffPath,
			userErrorPath,
			execResults[i],
			testCases[i].TimeLimitMs,
			testCases[i].MemoryLimitKB,
			(i + 1),
		)
	}

	finalMessage, finalStatus := v.constructFinalMessage(solutionStatuses, solutionMessages)

	v.logger.Infof("Evaluation completed [MsgID: %s]: %s", messageID, finalMessage)

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
		var b strings.Builder
		for i, message := range messages {
			if i != 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%d. %s", i+1, message)
		}
		b.WriteString(".")
		finalMessage = b.String()
	}
	return finalMessage, finalStatus
}

func (v *verifier) readExecutionResultFiles(
	execResultDirPath string,
	numberOfTest int,
) ([]*executor.ExecutionResult, error) {
	results := make([]*executor.ExecutionResult, numberOfTest)
	for i := range results {
		filePath := filepath.Join(execResultDirPath, fmt.Sprintf("%d.%s", i+1, constants.ExecutionResultFileExt))
		file, err := os.Open(filePath)
		if err != nil {
			v.logger.Errorf("Failed to open execution result file %s: %s", filePath, err)
			return nil, err
		}

		var exitCode int
		var execTime float64
		var peakMem int64
		_, err = fmt.Fscanf(file, "%d %f %d\n", &exitCode, &execTime, &peakMem)
		if err != nil {
			v.logger.Errorf("Failed to read line %d: %s", i, err)
			file.Close()
			return nil, err
		}
		results[i] = &executor.ExecutionResult{
			ExitCode: exitCode,
			ExecTime: execTime,
			PeakMem:  peakMem,
		}
		if err := file.Close(); err != nil {
			v.logger.Errorf("Failed to close execution result file %s: %s", filePath, err)
		}
	}
	return results, nil
}

func (v *verifier) evaluateTestCase(
	outputPath string,
	expectedOutputPath string,
	diffPath string,
	errorPath string,
	execResult *executor.ExecutionResult,
	timeLimit int64,
	memoryLimit int64,
	testCaseIdx int,

) (solution.TestResult, solution.ResultStatus, string) {
	solutionStatus := solution.Success
	solutionMessage := constants.SolutionMessageSuccess

	switch execResult.ExitCode {
	case constants.ExitCodeSuccess:
		passed, err := v.compareOutput(outputPath, expectedOutputPath, diffPath, errorPath)
		if err != nil {
			v.logger.Errorf("Error comparing output for test case %d: %s", testCaseIdx, err.Error())
			return solution.TestResult{
				Passed:        false,
				ExecutionTime: execResult.ExecTime,
				PeakMem:       execResult.PeakMem,
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
			PeakMem:       execResult.PeakMem,
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
			PeakMem:       execResult.PeakMem,
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
			PeakMem:       execResult.PeakMem,
			StatusCode:    solution.MemoryLimitExceeded,
			ErrorMessage:  message,
			Order:         testCaseIdx,
		}, solution.TestFailed, constants.SolutionMessageMemoryLimitExceeded

	default:
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			PeakMem:       execResult.PeakMem,
			StatusCode:    solution.NonZeroExitCode,
			ErrorMessage:  fmt.Sprintf(constants.TestCaseMessageNonZeroExitCode, execResult.ExitCode),
			Order:         testCaseIdx,
		}, solution.TestFailed, constants.SolutionMessageNonZeroExitCode
	}
}

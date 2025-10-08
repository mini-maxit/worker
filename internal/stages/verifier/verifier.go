package verifier

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
	CompareOutput(outputPath, expectedOutputPath, stderrPath string) (bool, error)
}

type DefaultVerifier struct {
	flags []string
	logger *zap.SugaredLogger
}

type TaskForEvaluation struct {
	taskFilesDirPath string
	outputDirName    string
	timeLimit        int
	memoryLimit      int
}

// CompareOutput compares the output file with the expected output file.
// It returns true if the files are the same, false otherwise.
// It returns a string of differences if there are any.
// It returns an error if any issue occurs during the comparison.
func (dv *DefaultVerifier) CompareOutput(outputPath, expectedFilePath, stderrPath string) (bool, error) {
	outputPath = filepath.Clean(outputPath)
	expectedFilePath = filepath.Clean(expectedFilePath)

	args := dv.flags

	args = append(args, outputPath, expectedFilePath)
	diffCmd := exec.Command("diff", args...)

	diffCmdOutput, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, fmt.Errorf("error opening output file %s: %w", outputPath, err)
	}
	defer diffCmdOutput.Close()

	diffCmd.Stdout = diffCmdOutput
	err = diffCmd.Run()
	diffExitCode := diffCmd.ProcessState.ExitCode()
	if err != nil {
		if diffExitCode == constants.ExitCodeDifference {
			return false, nil
		}
		return false, fmt.Errorf("error running diff command: %w", err)
	}

	return true, nil
}

func NewDefaultVerifier(flags []string) Verifier {
	return &DefaultVerifier{
		flags: flags,
	}
}

func (dv *DefaultVerifier) evaluateAllTestCases(dc *packager.TaskDirConfig, messageID string, inputFiles []string) solution.Result {
	testCases := make([]solution.TestResult, len(inputFiles))
	solutionStatuses := make([]solution.ResultStatus, len(inputFiles))
	solutionMessages := make([]string, len(inputFiles))

	execResults, err := dv.ReadExecutionResultFiles(dc.UserExecResultDirPath, len(inputFiles))
	if err != nil {
		dv.logger.Errorf("Error reading execution result file [MsgID: %s]: %s", messageID, err.Error())
		return solution.Result{
			OutputDir:  dc.UserOutputDirPath,
			StatusCode: solution.InternalError,
			Message:    err.Error(),
		}
	}

	for i := range inputFiles {
		outputPath := fmt.Sprintf("%s/%d.out", dc.UserOutputDirPath, (i + 1))
		stderrPath := fmt.Sprintf("%s/%d.err", dc.UserErrorDirPath, (i + 1))

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
	finalStatus := solution.Success
	for _, status := range solutionStatuses {
		if status != solution.Success {
			finalStatus = solution.TestFailed
			break
		}
	}

	if finalStatus == solution.Success {
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

	return solution.Result{
		OutputDir:   task.userOutputDirName,
		StatusCode:  finalStatus,
		Message:     finalMessage,
		TestResults: testCases,
	}
}

func (dv *DefaultVerifier) ReadExecutionResultFiles(
	execResultDirPath string,
	numberOfTest int,
) ([]*executor.ExecutionResult, error) {
	results := make([]*executor.ExecutionResult, numberOfTest)
	for i := range results {

		filePath := filepath.Join(execResultDirPath, fmt.Sprintf("%d.res", i+1))
		file, err := os.Open(filePath)
		if err != nil {
			dv.logger.Errorf("Failed to open execution result file %s: %s", filePath, err)
			return nil, err
		}
		defer file.Close()

		var exitCode int
		var execTime float64
		_, err = fmt.Fscanf(file, "%d %f\n", &exitCode, &execTime)
		if err != nil {
			dv.logger.Errorf("Failed to read line %d: %s", i, err)
			return nil, err
		}
		results[i] = &executor.ExecutionResult{
			ExitCode: exitCode,
			ExecTime: execTime,
		}
	}
	return results, nil
}

func (r *DefaultVerifier) evaluateTestCase(
	execResult *executor.ExecutionResult,
	dirConfig *packager.TaskDirConfig,
	testCase int,
) (solution.TestResult, solution.ResultStatus, string) {
	solutionStatus := solution.Success
	solutionMessage := constants.SolutionMessageSuccess

	switch execResult.ExitCode {
	case constants.ExitCodeSuccess:
		expectedFilePath := fmt.Sprintf("%s/%s/%d.out", task.taskFilesDirPath, task.outputDirName, testCase)

		passed, err := verifier.CompareOutput(outputPath, expectedFilePath, stderrPath)
		if err != nil {
			return solution.TestResult{
				Passed:        false,
				ExecutionTime: execResult.ExecTime,
				StatusCode:    solution.TestCaseStatus(solution.InternalError),
				ErrorMessage:  err.Error(),
				Order:         testCase,
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
			Order:         testCase,
		}, solutionStatus, solutionMessage

	case constants.ExitCodeTimeLimitExceeded:
		message := fmt.Sprintf(
			constants.TestCaseMessageTimeOut,
			task.timeLimit,
		)
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    solution.TimeLimitExceeded,
			ErrorMessage:  message,
			Order:         testCase,
		}, solution.TestFailed, constants.SolutionMessageTimeout

	case constants.ExitCodeMemoryLimitExceeded:
		message := fmt.Sprintf(
			constants.TestCaseMessageMemoryLimitExceeded,
			task.memoryLimit,
		)
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    solution.MemoryLimitExceeded,
			ErrorMessage:  message,
			Order:         testCase,
		}, solution.TestFailed, constants.SolutionMessageMemoryLimitExceeded

	default:
		return solution.TestResult{
			Passed:        false,
			ExecutionTime: execResult.ExecTime,
			StatusCode:    solution.RuntimeError,
			ErrorMessage:  constants.TestCaseMessageRuntimeError,
			Order:         testCase,
		}, solution.TestFailed, constants.SolutionMessageRuntimeError
	}
}


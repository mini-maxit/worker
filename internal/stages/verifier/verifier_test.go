package verifier

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	"github.com/mini-maxit/worker/tests"
)

func TestEvaluateAllTestCases_AllPass(t *testing.T) {
	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	expectedOutDir := filepath.Join(dir, "expected")
	userDiffDir := filepath.Join(dir, "userDiff")
	userErrDir := filepath.Join(dir, "userErr")
	execResDir := filepath.Join(dir, "execRes")

	// prepare files: expected and user output identical
	tests.WriteFile(t, expectedOutDir, "out.txt", "hello\n")
	tests.WriteFile(t, userOutDir, "out.txt", "hello\n")
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "0 0.100\n")

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath:     userOutDir,
		OutputDirPath:         expectedOutDir,
		UserDiffDirPath:       userDiffDir,
		UserErrorDirPath:      userErrDir,
		UserExecResultDirPath: execResDir,
	}

	ver := NewVerifier([]string{})

	tc := messages.TestCase{
		StdOutResult:   messages.FileLocation{Path: "out.txt"},
		ExpectedOutput: messages.FileLocation{Path: "out.txt"}}

	res := ver.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg1")
	if res.StatusCode != solution.Success {
		t.Fatalf("expected success, got: %v, message: %s", res.StatusCode, res.Message)
	}
	if len(res.TestResults) != 1 || !res.TestResults[0].Passed {
		t.Fatalf("expected test passed, got %+v", res.TestResults)
	}
	if res.Message != constants.SolutionMessageSuccess {
		t.Fatalf("expected message %q, got %q", constants.SolutionMessageSuccess, res.Message)
	}
}

func TestEvaluateAllTestCases_OutputDifference(t *testing.T) {
	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	expectedOutDir := filepath.Join(dir, "expected")
	userDiffDir := filepath.Join(dir, "userDiff")
	userErrDir := filepath.Join(dir, "userErr")
	execResDir := filepath.Join(dir, "execRes")

	tests.WriteFile(t, expectedOutDir, "out.txt", "hello\n")
	tests.WriteFile(t, userOutDir, "out.txt", "hi\n")
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "0 0.050\n")

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath:     userOutDir,
		OutputDirPath:         expectedOutDir,
		UserDiffDirPath:       userDiffDir,
		UserErrorDirPath:      userErrDir,
		UserExecResultDirPath: execResDir,
	}

	ver := NewVerifier([]string{})
	tc := messages.TestCase{
		StdOutResult:   messages.FileLocation{Path: "out.txt"},
		ExpectedOutput: messages.FileLocation{Path: "out.txt"}}
	res := ver.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg2")
	if res.StatusCode != solution.TestFailed {
		t.Fatalf("expected test failed, got: %v", res.StatusCode)
	}
	if res.TestResults[0].StatusCode != solution.OutputDifference {
		t.Fatalf("expected output difference code, got: %v", res.TestResults[0].StatusCode)
	}
	expectedMsg := "1. " + constants.SolutionMessageOutputDifference + "."
	if res.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, res.Message)
	}
}

func TestEvaluateAllTestCases_TimeAndMemoryAndRuntime(t *testing.T) {
	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	expectedOutDir := filepath.Join(dir, "expected")
	userDiffDir := filepath.Join(dir, "userDiff")
	userErrDir := filepath.Join(dir, "userErr")
	execResDir := filepath.Join(dir, "execRes")

	tests.WriteFile(t, expectedOutDir, "out.txt", "whatever\n")
	tests.WriteFile(t, userOutDir, "out.txt", "whatever\n")
	// time limit exceeded (exit code 143)
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "143 0.0\n")

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath:     userOutDir,
		OutputDirPath:         expectedOutDir,
		UserDiffDirPath:       userDiffDir,
		UserErrorDirPath:      userErrDir,
		UserExecResultDirPath: execResDir,
	}
	ver := NewVerifier([]string{})
	tc := messages.TestCase{
		StdOutResult:   messages.FileLocation{Path: "out.txt"},
		ExpectedOutput: messages.FileLocation{Path: "out.txt"},
		TimeLimitMs:    5,
	}
	res := ver.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg3")
	if res.TestResults[0].StatusCode != solution.TimeLimitExceeded {
		t.Fatalf("expected time limit status, got: %v", res.TestResults[0].StatusCode)
	}
	expectedMsg := "1. " + constants.SolutionMessageTimeout + "."
	if res.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, res.Message)
	}

	// memory limit exceeded (exit code 134)
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "134 0.0\n")
	res = ver.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg4")
	if res.TestResults[0].StatusCode != solution.MemoryLimitExceeded {
		t.Fatalf("expected memory limit status, got: %v", res.TestResults[0].StatusCode)
	}
	expectedMsg = "1. " + constants.SolutionMessageMemoryLimitExceeded + "."
	if res.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, res.Message)
	}

	// runtime error (exit code 2)
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "2 0.0\n")
	res = ver.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg5")
	if res.TestResults[0].StatusCode != solution.RuntimeError {
		t.Fatalf("expected runtime error status, got: %v", res.TestResults[0].StatusCode)
	}
	expectedMsg = "1. " + constants.SolutionMessageRuntimeError + "."
	if res.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, res.Message)
	}
}

func TestEvaluateAllTestCases_MissingExecResult(t *testing.T) {
	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	expectedOutDir := filepath.Join(dir, "expected")
	userDiffDir := filepath.Join(dir, "userDiff")
	userErrDir := filepath.Join(dir, "userErr")
	execResDir := filepath.Join(dir, "execRes")

	tests.WriteFile(t, expectedOutDir, "out.txt", "a\n")
	tests.WriteFile(t, userOutDir, "out.txt", "a\n")
	// do NOT create exec result files

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath:     userOutDir,
		OutputDirPath:         expectedOutDir,
		UserDiffDirPath:       userDiffDir,
		UserErrorDirPath:      userErrDir,
		UserExecResultDirPath: execResDir,
	}
	ver := NewVerifier([]string{})
	tc := messages.TestCase{
		StdOutResult:   messages.FileLocation{Path: "out.txt"},
		ExpectedOutput: messages.FileLocation{Path: "out.txt"}}
	res := ver.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg6")
	if res.StatusCode != solution.InternalError {
		t.Fatalf("expected internal error due to missing exec result, got: %v", res.StatusCode)
	}
	if res.Message == "" {
		t.Fatalf("expected non-empty error message when exec result missing")
	}
}

func TestEvaluateAllTestCases_WithFlags_IgnoreWhitespace(t *testing.T) {
	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	expectedOutDir := filepath.Join(dir, "expected")
	userDiffDir := filepath.Join(dir, "userDiff")
	userErrDir := filepath.Join(dir, "userErr")
	execResDir := filepath.Join(dir, "execRes")

	// expected and user differ only by whitespace
	tests.WriteFile(t, expectedOutDir, "out.txt", "hello\n")
	tests.WriteFile(t, userOutDir, "out.txt", "hello \n")
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "0 0.010\n")

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath:     userOutDir,
		OutputDirPath:         expectedOutDir,
		UserDiffDirPath:       userDiffDir,
		UserErrorDirPath:      userErrDir,
		UserExecResultDirPath: execResDir,
	}

	// without flags -> should detect difference
	verNoFlags := NewVerifier([]string{})
	tc := messages.TestCase{
		StdOutResult:   messages.FileLocation{Path: "out.txt"},
		ExpectedOutput: messages.FileLocation{Path: "out.txt"}}
	res := verNoFlags.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg-flags-1")
	if res.StatusCode != solution.TestFailed {
		t.Fatalf("expected test failed without flags, got: %v", res.StatusCode)
	}
	expectedMsg := "1. " + constants.SolutionMessageOutputDifference + "."
	if res.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, res.Message)
	}

	// with -w flag (ignore whitespace) -> should pass
	verIgnoreWS := NewVerifier([]string{"-w"})
	res2 := verIgnoreWS.EvaluateAllTestCases(cfg, []messages.TestCase{tc}, "msg-flags-2")
	if res2.StatusCode != solution.Success {
		t.Fatalf("expected success with -w flag, got: %v, message: %s", res2.StatusCode, res2.Message)
	}
	if res2.Message != constants.SolutionMessageSuccess {
		t.Fatalf("expected message %q with -w flag, got %q", constants.SolutionMessageSuccess, res2.Message)
	}
}

func TestEvaluateAllTestCases_MultipleStatuses(t *testing.T) {
	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	expectedOutDir := filepath.Join(dir, "expected")
	userDiffDir := filepath.Join(dir, "userDiff")
	userErrDir := filepath.Join(dir, "userErr")
	execResDir := filepath.Join(dir, "execRes")

	// Test case 1: passed
	tests.WriteFile(t, expectedOutDir, "t1.txt", "ok\n")
	tests.WriteFile(t, userOutDir, "t1.txt", "ok\n")

	// Test case 2: output difference
	tests.WriteFile(t, expectedOutDir, "t2.txt", "one\n")
	tests.WriteFile(t, userOutDir, "t2.txt", "two\n")

	// Test case 3: runtime error (exit code != 0 and not mem/time)
	tests.WriteFile(t, expectedOutDir, "t3.txt", "x\n")
	tests.WriteFile(t, userOutDir, "t3.txt", "x\n")

	// Test case 4: memory limit exceeded
	tests.WriteFile(t, expectedOutDir, "t4.txt", "x\n")
	tests.WriteFile(t, userOutDir, "t4.txt", "x\n")

	// Test case 5: time limit exceeded
	tests.WriteFile(t, expectedOutDir, "t5.txt", "x\n")
	tests.WriteFile(t, userOutDir, "t5.txt", "x\n")

	// exec results: 1 -> 0 (pass), 2 -> 0 (diff), 3 -> 2 (runtime), 4 -> 134 (mem), 5 -> 143 (time)
	tests.WriteFile(t, execResDir, "1."+constants.ExecutionResultFileExt, "0 0.001\n")
	tests.WriteFile(t, execResDir, "2."+constants.ExecutionResultFileExt, "0 0.002\n")
	tests.WriteFile(t, execResDir, "3."+constants.ExecutionResultFileExt, "2 0.003\n")
	tests.WriteFile(t, execResDir, "4."+constants.ExecutionResultFileExt, "134 0.004\n")
	tests.WriteFile(t, execResDir, "5."+constants.ExecutionResultFileExt, "143 0.005\n")

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath:     userOutDir,
		OutputDirPath:         expectedOutDir,
		UserDiffDirPath:       userDiffDir,
		UserErrorDirPath:      userErrDir,
		UserExecResultDirPath: execResDir,
	}

	ver := NewVerifier([]string{})

	tcs := []messages.TestCase{
		{StdOutResult: messages.FileLocation{Path: "t1.txt"}, ExpectedOutput: messages.FileLocation{Path: "t1.txt"}},
		{StdOutResult: messages.FileLocation{Path: "t2.txt"}, ExpectedOutput: messages.FileLocation{Path: "t2.txt"}},
		{StdOutResult: messages.FileLocation{Path: "t3.txt"}, ExpectedOutput: messages.FileLocation{Path: "t3.txt"}},
		{StdOutResult: messages.FileLocation{Path: "t4.txt"}, ExpectedOutput: messages.FileLocation{Path: "t4.txt"}},
		{StdOutResult: messages.FileLocation{Path: "t5.txt"}, ExpectedOutput: messages.FileLocation{Path: "t5.txt"}},
	}

	res := ver.EvaluateAllTestCases(cfg, tcs, "msg-multi")

	if res.StatusCode != solution.TestFailed {
		t.Fatalf("expected overall TestFailed, got: %v, message: %s", res.StatusCode, res.Message)
	}

	if len(res.TestResults) != 5 {
		t.Fatalf("expected 5 test results, got %d", len(res.TestResults))
	}

	// Validate individual statuses
	wantStatuses := []solution.TestCaseStatus{
		solution.TestCasePassed,
		solution.OutputDifference,
		solution.RuntimeError,
		solution.MemoryLimitExceeded,
		solution.TimeLimitExceeded,
	}
	wantPassed := []bool{true, false, false, false, false}
	for i := range 5 {
		if res.TestResults[i].StatusCode != wantStatuses[i] {
			t.Fatalf("test %d: expected status %v, got %v", i+1, wantStatuses[i], res.TestResults[i].StatusCode)
		}
		if res.TestResults[i].Passed != wantPassed[i] {
			t.Fatalf("test %d: expected passed %v, got %v", i+1, wantPassed[i], res.TestResults[i].Passed)
		}
	}

	// Build expected final message: include all messages in order
	msgs := []string{
		constants.SolutionMessageSuccess,
		constants.SolutionMessageOutputDifference,
		constants.SolutionMessageRuntimeError,
		constants.SolutionMessageMemoryLimitExceeded,
		constants.SolutionMessageTimeout,
	}
	expectedMsg := fmt.Sprintf("1. %s, 2. %s, 3. %s, 4. %s, 5. %s.", msgs[0], msgs[1], msgs[2], msgs[3], msgs[4])
	if res.Message != expectedMsg {
		t.Fatalf("expected message %q, got %q", expectedMsg, res.Message)
	}
}

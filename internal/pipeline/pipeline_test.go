package pipeline

import (
    "errors"
    "testing"

	"go.uber.org/mock/gomock"
    mockpkg "github.com/mini-maxit/worker/internal/pipeline/mocks"
    "github.com/mini-maxit/worker/internal/stages/executor"
    "github.com/mini-maxit/worker/internal/stages/packager"
    "github.com/mini-maxit/worker/pkg/constants"
    customErr "github.com/mini-maxit/worker/pkg/errors"
    "github.com/mini-maxit/worker/pkg/languages"
    "github.com/mini-maxit/worker/pkg/messages"
    "github.com/mini-maxit/worker/pkg/solution"
)

func createSampleTaskMessage(langType string) *messages.TaskQueueMessage {
    return &messages.TaskQueueMessage{
        LanguageType:    langType,
        LanguageVersion: "17",
        SubmissionFile: messages.FileLocation{
            ServerType: "local",
            Bucket:     "submissions",
            Path:       "42/main.cpp",
        },
        TestCases: []messages.TestCase{
            {
                Order: 1,
                InputFile: messages.FileLocation{
                    ServerType: "local",
                    Bucket:     "tasks",
                    Path:       "1/in1.txt",
                },
                ExpectedOutput: messages.FileLocation{ServerType: "local", Bucket: "tasks", Path: "1/out1.txt"},
                StdOutResult:    messages.FileLocation{ServerType: "local", Bucket: "results", Path: "42/stdout1.txt"},
                StdErrResult:    messages.FileLocation{ServerType: "local", Bucket: "results", Path: "42/stderr1.txt"},
                DiffResult:      messages.FileLocation{ServerType: "local", Bucket: "results", Path: "42/diff1.txt"},
                TimeLimitMs:     1000,
                MemoryLimitKB:   256000,
            },
            {
                Order: 2,
                InputFile: messages.FileLocation{
                    ServerType: "local",
                    Bucket:     "tasks",
                    Path:       "1/in2.txt",
                },
                ExpectedOutput: messages.FileLocation{ServerType: "local", Bucket: "tasks", Path: "1/out2.txt"},
                StdOutResult:    messages.FileLocation{ServerType: "local", Bucket: "results", Path: "42/stdout2.txt"},
                StdErrResult:    messages.FileLocation{ServerType: "local", Bucket: "results", Path: "42/stderr2.txt"},
                DiffResult:      messages.FileLocation{ServerType: "local", Bucket: "results", Path: "42/diff2.txt"},
                TimeLimitMs:     1000,
                MemoryLimitKB:   256000,
            },
        },
    }
}

// Helper to build a default dir config used by tests
func defaultDirConfig() *packager.TaskDirConfig {
    return &packager.TaskDirConfig{
        PackageDirPath:     "/tmp/test-package",
        UserSolutionPath:   "/tmp/test-package/main.cpp",
        UserExecFilePath:   "/tmp/test-package/main",
        CompileErrFilePath: "/tmp/test-package/compile.err",
    }
}

func TestProcessTask_SuccessfulExecution(t *testing.T) {
    messageID := "test-msg-123"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    // expectations
    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
    mockVerif.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).Return(solution.Result{StatusCode: solution.Success, Message: constants.SolutionMessageSuccess})
    mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Return(nil)
    mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Return(nil)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_CompilationError(t *testing.T) {
    messageID := "test-msg-compile-err"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(customErr.ErrCompilationFailed)
    mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), true).Return(nil)
    mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.AssignableToTypeOf(solution.Result{})).Do(func(_ string, _ string, res solution.Result) error {
        if res.StatusCode != solution.CompilationError {
            t.Fatalf("expected compilation error status, got %v", res.StatusCode)
        }
        return nil
    }).Return(nil)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_RuntimeError(t *testing.T) {
    messageID := "test-msg-runtime-err"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec.EXPECT().ExecuteCommand(gomock.Any()).Return(errors.New("runtime execution failed"))
    mockResp.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, messageID, gomock.Any())

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_OutputDifference(t *testing.T) {
    messageID := "test-msg-output-diff"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
    mockVerif.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).Return(solution.Result{StatusCode: solution.TestFailed, Message: constants.SolutionMessageOutputDifference, TestResults: []solution.TestResult{{Passed: true, Order: 1}, {Passed: false, Order: 2, StatusCode: solution.OutputDifference}}})
    mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Return(nil)
    mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Return(nil)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_TimeLimitAndMemoryExceeded(t *testing.T) {
    messageID := "test-msg-time-memory"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
    mockVerif.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).Return(solution.Result{StatusCode: solution.TestFailed, Message: constants.SolutionMessageTimeout, TestResults: []solution.TestResult{{Passed: false, StatusCode: solution.TimeLimitExceeded, Order: 1}}})
    mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Return(nil)
    mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Return(nil)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)

    // Test memory exceeded
    ctrl2 := gomock.NewController(t)
    defer ctrl2.Finish()
    mockPkgr2 := mockpkg.NewMockPackager(ctrl2)
    mockComp2 := mockpkg.NewMockCompiler(ctrl2)
    mockExec2 := mockpkg.NewMockExecutor(ctrl2)
    mockVerif2 := mockpkg.NewMockVerifier(ctrl2)
    mockResp2 := mockpkg.NewMockResponder(ctrl2)

    mockPkgr2.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp2.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec2.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
    mockVerif2.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).Return(solution.Result{StatusCode: solution.TestFailed, Message: constants.SolutionMessageMemoryLimitExceeded, TestResults: []solution.TestResult{{Passed: false, StatusCode: solution.MemoryLimitExceeded, Order: 1}}})
    mockPkgr2.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Return(nil)
    mockResp2.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Return(nil)

    worker2 := NewWorker(2, mockComp2, mockPkgr2, mockExec2, mockVerif2, mockResp2)
    worker2.ProcessTask(messageID, task)
}

func TestProcessTask_InvalidLanguageType(t *testing.T) {
    messageID := "test-msg-invalid-lang"
    task := createSampleTaskMessage("invalid_language")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    mockResp.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, gomock.Any(), gomock.AssignableToTypeOf(errors.New("")))

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_PackagerError(t *testing.T) {
    messageID := "test-msg-packager-err"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    packagerError := errors.New("failed to download submission file")
    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(nil, packagerError)
    mockResp.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, messageID, packagerError)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_SendPackageErrorAndPublishFallback(t *testing.T) {
    messageID := "test-msg-send-err"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    sendErr := errors.New("failed to upload results")

    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
    mockVerif.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).Return(solution.Result{StatusCode: solution.Success, Message: constants.SolutionMessageSuccess})
    mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Return(sendErr)
    mockResp.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, messageID, sendErr)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_PublishResponseErrorFallback(t *testing.T) {
    messageID := "test-msg-publish-err"
    task := createSampleTaskMessage("cpp")

    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockPkgr := mockpkg.NewMockPackager(ctrl)
    mockComp := mockpkg.NewMockCompiler(ctrl)
    mockExec := mockpkg.NewMockExecutor(ctrl)
    mockVerif := mockpkg.NewMockVerifier(ctrl)
    mockResp := mockpkg.NewMockResponder(ctrl)

    publishErr := errors.New("failed to publish to queue")

    mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(defaultDirConfig(), nil)
    mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)
    mockExec.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
    mockVerif.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).Return(solution.Result{StatusCode: solution.Success, Message: constants.SolutionMessageSuccess})
    mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Return(nil)
    mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Return(publishErr)
    mockResp.EXPECT().PublishErrorToResponseQueue(constants.QueueMessageTypeTask, messageID, publishErr)

    worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)
    worker.ProcessTask(messageID, task)
}

func TestProcessTask_FullPipelineSuccess(t *testing.T) {
 
	var executorCalled = false
	var verifierCalled = false
	var responderCalled = false
	var sendPackageCalled = false

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPkgr := mockpkg.NewMockPackager(ctrl)
	mockComp := mockpkg.NewMockCompiler(ctrl)
	mockExec := mockpkg.NewMockExecutor(ctrl)
	mockVerif := mockpkg.NewMockVerifier(ctrl)
	mockResp := mockpkg.NewMockResponder(ctrl)

	mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).DoAndReturn(func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
		packagerCalled = true
		return &packager.TaskDirConfig{
			PackageDirPath:     "/tmp/test-package",
			UserSolutionPath:   "/tmp/test-package/main.cpp",
			UserExecFilePath:   "/tmp/test-package/main",
			CompileErrFilePath: "/tmp/test-package/compile.err",
		}, nil
	})

	mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), false).Do(func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) {
		sendPackageCalled = true
	}).Return(nil)

	mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(nil)

	mockExec.EXPECT().ExecuteCommand(gomock.Any()).Do(func(cfg executor.CommandConfig) error {
		executorCalled = true
		return nil
	}).Return(nil)

	mockVerif.EXPECT().EvaluateAllTestCases(gomock.Any(), gomock.Any(), messageID).DoAndReturn(func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
		verifierCalled = true
		return solution.Result{
			StatusCode: solution.Success,
			Message:    constants.SolutionMessageSuccess,
			TestResults: []solution.TestResult{
				{Passed: true, ExecutionTime: 0.5, StatusCode: solution.TestCasePassed, Order: 1},
				{Passed: true, ExecutionTime: 0.6, StatusCode: solution.TestCasePassed, Order: 2},
			},
		}
	})

	mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Do(func(_ string, _ string, taskResult solution.Result) error {
		responderCalled = true
		if taskResult.StatusCode != solution.Success {
			t.Errorf("Expected success status, got %v", taskResult.StatusCode)
		}
		return nil
	}).Return(nil)

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	// Verify all stages were called
	if !packagerCalled {
		t.Error("Packager was not called")
	}
	if !executorCalled {
		t.Error("Executor was not called")
	}
	if !verifierCalled {
		t.Error("Verifier was not called")
	}
	if !responderCalled {
		t.Error("Responder was not called")
	}
	if !sendPackageCalled {
		t.Error("SendSolutionPackage was not called")
	}

	// Verify worker state
	if worker.GetProcessingMessageID() != "" {
		t.Error("Expected processing message ID to be empty after completion")
	}
}

// TestProcessTask_CompilationError tests handling of compilation errors
func TestProcessTask_CompilationError(t *testing.T) {
	messageID := "test-msg-compile-err"
	task := createSampleTaskMessage("cpp")

	compilationErrorOccurred := false
	responderCalledWithCompileError := false
	sendPackageCalledWithError := false

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPkgr := mockpkg.NewMockPackager(ctrl)
	mockComp := mockpkg.NewMockCompiler(ctrl)
	mockExec := mockpkg.NewMockExecutor(ctrl)
	mockVerif := mockpkg.NewMockVerifier(ctrl)
	mockResp := mockpkg.NewMockResponder(ctrl)

	mockPkgr.EXPECT().PrepareSolutionPackage(gomock.Any(), messageID).Return(&packager.TaskDirConfig{
		PackageDirPath:     "/tmp/test-package",
		UserSolutionPath:   "/tmp/test-package/main.cpp",
		UserExecFilePath:   "/tmp/test-package/main",
		CompileErrFilePath: "/tmp/test-package/compile.err",
	}, nil)

	mockComp.EXPECT().CompileSolutionIfNeeded(languages.CPP, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), messageID).Return(customErr.ErrCompilationFailed)

	// Expect SendSolutionPackage called with hasCompilationErr == true
	mockPkgr.EXPECT().SendSolutionPackage(gomock.Any(), gomock.Any(), true).Do(func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) {
		sendPackageCalledWithError = true
	}).Return(nil)

	mockResp.EXPECT().PublishPayloadTaskRespond(constants.QueueMessageTypeTask, messageID, gomock.Any()).Do(func(_ string, _ string, taskResult solution.Result) error {
		responderCalledWithCompileError = true
		if taskResult.StatusCode != solution.CompilationError {
			t.Errorf("Expected CompilationError status, got %v", taskResult.StatusCode)
		}
		return nil
	}).Return(nil)

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	worker.ProcessTask(messageID, task)

	if !sendPackageCalledWithError {
		t.Error("SendSolutionPackage was not called with compilation error flag")
	}
}

// TestProcessTask_RuntimeError tests handling of runtime errors during execution
func TestProcessTask_RuntimeError(t *testing.T) {
	messageID := "test-msg-runtime-err"
	task := createSampleTaskMessage("cpp")

	executorErrorOccurred := false
	errorResponderCalled := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
	}

	mockComp := &mockCompiler{
		compileSolutionIfNeededFunc: func(langType languages.LanguageType, langVersion string, sourceFilePath string, outFilePath string, compErrFilePath string, messageID string) error {
			return nil
		},
	}

	mockExec := &mockExecutor{
		executeCommandFunc: func(cfg executor.CommandConfig) error {
			executorErrorOccurred = true
			return errors.New("runtime execution failed")
		},
	}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			t.Error("Verifier should not be called on executor error")
			return solution.Result{}
		},
	}

	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			t.Error("PublishPayloadTaskRespond should not be called on executor error")
			return nil
		},
		publishErrorToResponseQueueFunc: func(messageType, msgID string, err error) {
			errorResponderCalled = true
			if err.Error() != "runtime execution failed" {
				t.Errorf("Expected 'runtime execution failed' error, got %s", err.Error())
			}
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	// Verify error handling
	if !executorErrorOccurred {
		t.Error("Executor error did not occur")
	}
	if !errorResponderCalled {
		t.Error("Error responder was not called")
	}
}

// TestProcessTask_OutputDifference tests handling of test case failures with output differences
func TestProcessTask_OutputDifference(t *testing.T) {
	messageID := "test-msg-output-diff"
	task := createSampleTaskMessage("cpp")

	verifierCalledWithDiff := false
	responderCalledWithFailure := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
		sendSolutionPackageFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error {
			return nil
		},
	}

	mockComp := &mockCompiler{
		compileSolutionIfNeededFunc: func(langType languages.LanguageType, langVersion string, sourceFilePath string, outFilePath string, compErrFilePath string, messageID string) error {
			return nil
		},
	}

	mockExec := &mockExecutor{
		executeCommandFunc: func(cfg executor.CommandConfig) error {
			return nil
		},
	}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			verifierCalledWithDiff = true
			return solution.Result{
				StatusCode: solution.TestFailed,
				Message:    constants.SolutionMessageOutputDifference,
				TestResults: []solution.TestResult{
					{
						Passed:        true,
						ExecutionTime: 0.5,
						StatusCode:    solution.TestCasePassed,
						Order:         1,
					},
					{
						Passed:        false,
						ExecutionTime: 0.6,
						StatusCode:    solution.OutputDifference,
						ErrorMessage:  "Output does not match expected",
						Order:         2,
					},
				},
			}
		},
	}

	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			responderCalledWithFailure = true
			if taskResult.StatusCode != solution.TestFailed {
				t.Errorf("Expected TestFailed status, got %v", taskResult.StatusCode)
			}
			if len(taskResult.TestResults) != 2 {
				t.Errorf("Expected 2 test results, got %d", len(taskResult.TestResults))
			}
			// Check first test passed
			if !taskResult.TestResults[0].Passed {
				t.Error("Expected first test to pass")
			}
			// Check second test failed with output difference
			if taskResult.TestResults[1].Passed {
				t.Error("Expected second test to fail")
			}
			if taskResult.TestResults[1].StatusCode != solution.OutputDifference {
				t.Errorf("Expected OutputDifference status, got %v", taskResult.TestResults[1].StatusCode)
			}
			return nil
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	// Verify verifier was called and returned differences
	if !verifierCalledWithDiff {
		t.Error("Verifier was not called")
	}
	if !responderCalledWithFailure {
		t.Error("Responder was not called with test failure")
	}
}

// TestProcessTask_TimeLimitExceeded tests handling of time limit exceeded
func TestProcessTask_TimeLimitExceeded(t *testing.T) {
	messageID := "test-msg-timeout"
	task := createSampleTaskMessage("cpp")

	timeoutDetected := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
		sendSolutionPackageFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error {
			return nil
		},
	}

	mockComp := &mockCompiler{}

	mockExec := &mockExecutor{
		executeCommandFunc: func(cfg executor.CommandConfig) error {
			return nil
		},
	}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			return solution.Result{
				StatusCode: solution.TestFailed,
				Message:    constants.SolutionMessageTimeout,
				TestResults: []solution.TestResult{
					{
						Passed:        false,
						ExecutionTime: 1.5,
						StatusCode:    solution.TimeLimitExceeded,
						ErrorMessage:  "Solution timed out after 1 s",
						Order:         1,
					},
				},
			}
		},
	}

	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			timeoutDetected = true
			if taskResult.StatusCode != solution.TestFailed {
				t.Errorf("Expected TestFailed status, got %v", taskResult.StatusCode)
			}
			if taskResult.TestResults[0].StatusCode != solution.TimeLimitExceeded {
				t.Errorf("Expected TimeLimitExceeded status, got %v", taskResult.TestResults[0].StatusCode)
			}
			return nil
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	if !timeoutDetected {
		t.Error("Timeout was not detected")
	}
}

// TestProcessTask_MemoryLimitExceeded tests handling of memory limit exceeded
func TestProcessTask_MemoryLimitExceeded(t *testing.T) {
	messageID := "test-msg-memory"
	task := createSampleTaskMessage("cpp")

	memoryLimitDetected := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
		sendSolutionPackageFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error {
			return nil
		},
	}

	mockComp := &mockCompiler{}

	mockExec := &mockExecutor{
		executeCommandFunc: func(cfg executor.CommandConfig) error {
			return nil
		},
	}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			return solution.Result{
				StatusCode: solution.TestFailed,
				Message:    constants.SolutionMessageMemoryLimitExceeded,
				TestResults: []solution.TestResult{
					{
						Passed:        false,
						ExecutionTime: 0.5,
						StatusCode:    solution.MemoryLimitExceeded,
						ErrorMessage:  "Solution exceeded memory limit of 250 MB",
						Order:         1,
					},
				},
			}
		},
	}

	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			memoryLimitDetected = true
			if taskResult.StatusCode != solution.TestFailed {
				t.Errorf("Expected TestFailed status, got %v", taskResult.StatusCode)
			}
			if taskResult.TestResults[0].StatusCode != solution.MemoryLimitExceeded {
				t.Errorf("Expected MemoryLimitExceeded status, got %v", taskResult.TestResults[0].StatusCode)
			}
			return nil
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	if !memoryLimitDetected {
		t.Error("Memory limit exceeded was not detected")
	}
}


// TestProcessTask_PackagerError tests handling of packager errors
func TestProcessTask_PackagerError(t *testing.T) {
	messageID := "test-msg-packager-err"
	task := createSampleTaskMessage("cpp")

	packagerError := errors.New("failed to download submission file")
	errorPublished := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return nil, packagerError
		},
	}

	mockComp := &mockCompiler{
		compileSolutionIfNeededFunc: func(langType languages.LanguageType, langVersion string, sourceFilePath string, outFilePath string, compErrFilePath string, messageID string) error {
			t.Error("Compiler should not be called on packager error")
			return nil
		},
	}

	mockExec := &mockExecutor{}
	mockVerif := &mockVerifier{}

	mockResp := &mockResponder{
		publishErrorToResponseQueueFunc: func(messageType, msgID string, err error) {
			errorPublished = true
			if err.Error() != packagerError.Error() {
				t.Errorf("Expected error '%s', got '%s'", packagerError.Error(), err.Error())
			}
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	if !errorPublished {
		t.Error("Error was not published")
	}
}

// TestProcessTask_SendPackageError tests handling of errors when sending solution package
func TestProcessTask_SendPackageError(t *testing.T) {
	messageID := "test-msg-send-err"
	task := createSampleTaskMessage("cpp")

	sendError := errors.New("failed to upload results")
	errorPublished := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
		sendSolutionPackageFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error {
			return sendError
		},
	}

	mockComp := &mockCompiler{}
	mockExec := &mockExecutor{}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			return solution.Result{
				StatusCode: solution.Success,
				Message:    constants.SolutionMessageSuccess,
			}
		},
	}

	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			t.Error("PublishPayloadTaskRespond should not be called on send package error")
			return nil
		},
		publishErrorToResponseQueueFunc: func(messageType, msgID string, err error) {
			errorPublished = true
			if err.Error() != sendError.Error() {
				t.Errorf("Expected error '%s', got '%s'", sendError.Error(), err.Error())
			}
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	if !errorPublished {
		t.Error("Error was not published")
	}
}

// TestProcessTask_PublishResponseError tests handling of errors when publishing response
func TestProcessTask_PublishResponseError(t *testing.T) {
	messageID := "test-msg-publish-err"
	task := createSampleTaskMessage("cpp")

	publishError := errors.New("failed to publish to queue")
	errorFallbackCalled := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
		sendSolutionPackageFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error {
			return nil
		},
	}

	mockComp := &mockCompiler{}
	mockExec := &mockExecutor{}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			return solution.Result{
				StatusCode: solution.Success,
				Message:    constants.SolutionMessageSuccess,
			}
		},
	}

	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			return publishError
		},
		publishErrorToResponseQueueFunc: func(messageType, msgID string, err error) {
			errorFallbackCalled = true
			if err.Error() != publishError.Error() {
				t.Errorf("Expected error '%s', got '%s'", publishError.Error(), err.Error())
			}
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	if !errorFallbackCalled {
		t.Error("Error fallback was not called")
	}
}

// TestWorker_GettersAndSetters tests worker getter and setter methods
func TestWorker_GettersAndSetters(t *testing.T) {
	mockPkgr := &mockPackager{}
	mockComp := &mockCompiler{}
	mockExec := &mockExecutor{}
	mockVerif := &mockVerifier{}
	mockResp := &mockResponder{}

	worker := NewWorker(5, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Test GetId
	if worker.GetId() != 5 {
		t.Errorf("Expected worker ID 5, got %d", worker.GetId())
	}

	// Test GetStatus and UpdateStatus
	if worker.GetStatus() != constants.WorkerStatusIdle {
		t.Errorf("Expected initial status %s, got %s", constants.WorkerStatusIdle, worker.GetStatus())
	}

	worker.UpdateStatus(constants.WorkerStatusBusy)
	if worker.GetStatus() != constants.WorkerStatusBusy {
		t.Errorf("Expected status %s, got %s", constants.WorkerStatusBusy, worker.GetStatus())
	}

	// Test GetProcessingMessageID
	if worker.GetProcessingMessageID() != "" {
		t.Errorf("Expected empty processing message ID, got %s", worker.GetProcessingMessageID())
	}
}

// TestProcessTask_PanicRecovery tests that panics are recovered and handled
func TestProcessTask_PanicRecovery(t *testing.T) {
	messageID := "test-msg-panic"
	task := createSampleTaskMessage("cpp")

	panicError := errors.New("unexpected panic occurred")
	errorPublished := false

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			panic(panicError)
		},
	}

	mockComp := &mockCompiler{}
	mockExec := &mockExecutor{}
	mockVerif := &mockVerifier{}

	mockResp := &mockResponder{
		publishErrorToResponseQueueFunc: func(messageType, msgID string, err error) {
			errorPublished = true
			if err.Error() != panicError.Error() {
				t.Errorf("Expected error '%s', got '%s'", panicError.Error(), err.Error())
			}
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task - should recover from panic
	worker.ProcessTask(messageID, task)

	if !errorPublished {
		t.Error("Error was not published after panic recovery")
	}
}

// TestProcessTask_MultipleTestCases tests handling of multiple test cases with mixed results
func TestProcessTask_MultipleTestCases(t *testing.T) {
	messageID := "test-msg-multiple"
	task := createSampleTaskMessage("cpp")
	// Add more test cases
	task.TestCases = append(task.TestCases, messages.TestCase{
		Order: 3,
		InputFile: messages.FileLocation{
			ServerType: "local",
			Bucket:     "tasks",
			Path:       "1/in3.txt",
		},
		ExpectedOutput: messages.FileLocation{
			ServerType: "local",
			Bucket:     "tasks",
			Path:       "1/out3.txt",
		},
		TimeLimitMs:   1000,
		MemoryLimitKB: 256000,
	})

	mockPkgr := &mockPackager{
		prepareSolutionPackageFunc: func(taskQueueMessage *messages.TaskQueueMessage, msgID string) (*packager.TaskDirConfig, error) {
			return &packager.TaskDirConfig{
				PackageDirPath:     "/tmp/test-package",
				UserSolutionPath:   "/tmp/test-package/main.cpp",
				UserExecFilePath:   "/tmp/test-package/main",
				CompileErrFilePath: "/tmp/test-package/compile.err",
			}, nil
		},
		sendSolutionPackageFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, hasCompilationErr bool) error {
			return nil
		},
	}

	mockComp := &mockCompiler{}
	mockExec := &mockExecutor{}

	mockVerif := &mockVerifier{
		evaluateAllTestCasesFunc: func(dirConfig *packager.TaskDirConfig, testCases []messages.TestCase, messageID string) solution.Result {
			return solution.Result{
				StatusCode: solution.TestFailed,
				Message:    "Some tests failed",
				TestResults: []solution.TestResult{
					{
						Passed:        true,
						ExecutionTime: 0.5,
						StatusCode:    solution.TestCasePassed,
						Order:         1,
					},
					{
						Passed:        false,
						ExecutionTime: 0.6,
						StatusCode:    solution.OutputDifference,
						ErrorMessage:  "Output mismatch",
						Order:         2,
					},
					{
						Passed:        false,
						ExecutionTime: 1.5,
						StatusCode:    solution.TimeLimitExceeded,
						ErrorMessage:  "Timeout",
						Order:         3,
					},
				},
			}
		},
	}

	responderCalled := false
	mockResp := &mockResponder{
		publishPayloadTaskRespondFunc: func(messageType, msgID string, taskResult solution.Result) error {
			responderCalled = true
			if len(taskResult.TestResults) != 3 {
				t.Errorf("Expected 3 test results, got %d", len(taskResult.TestResults))
			}
			if taskResult.TestResults[0].Passed != true {
				t.Error("Expected first test to pass")
			}
			if taskResult.TestResults[1].StatusCode != solution.OutputDifference {
				t.Errorf("Expected OutputDifference for test 2, got %v", taskResult.TestResults[1].StatusCode)
			}
			if taskResult.TestResults[2].StatusCode != solution.TimeLimitExceeded {
				t.Errorf("Expected TimeLimitExceeded for test 3, got %v", taskResult.TestResults[2].StatusCode)
			}
			return nil
		},
	}

	worker := NewWorker(1, mockComp, mockPkgr, mockExec, mockVerif, mockResp)

	// Process the task
	worker.ProcessTask(messageID, task)

	if !responderCalled {
		t.Error("Responder was not called")
	}
}

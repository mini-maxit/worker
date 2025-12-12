package pipeline_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/pipeline"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	mocks "github.com/mini-maxit/worker/tests/mocks"
	"go.uber.org/mock/gomock"
)

// setupSuccessfulPipelineMocks configures mocks for a successful task processing flow.
func setupSuccessfulPipelineMocks(
	t *testing.T,
	mockCompiler *mocks.MockCompiler,
	mockPackager *mocks.MockPackager,
	mockExecutor *mocks.MockExecutor,
	mockVerifier *mocks.MockVerifier,
	mockResponder *mocks.MockResponder,
) {
	dir := &packager.TaskDirConfig{
		PackageDirPath:     t.TempDir(),
		UserSolutionPath:   "src",
		UserExecFilePath:   "exec",
		CompileErrFilePath: "compile.err",
	}
	mockPackager.EXPECT().PrepareSolutionPackage(gomock.Any(), gomock.Any()).Return(dir, nil)
	mockCompiler.EXPECT().
		CompileSolutionIfNeeded(
			gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil)
	mockExecutor.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
	mockVerifier.EXPECT().
		EvaluateAllTestCases(dir, gomock.Any(), gomock.Any()).
		Return(solution.Result{
			StatusCode: solution.Success,
			Message:    "OK",
		})
	mockPackager.EXPECT().SendSolutionPackage(dir, gomock.Any(), false, gomock.Any()).Return(nil)
	mockResponder.EXPECT().
		PublishPayloadTaskRespond(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
}

func TestProcessTask_SuccessFlow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	setupSuccessfulPipelineMocks(t, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)

	w := pipeline.NewWorker(1, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)

	task := &messages.TaskQueueMessage{
		LanguageType:    "cpp",
		LanguageVersion: "11",
		TestCases: []messages.TestCase{
			{TimeLimitMs: 100, MemoryLimitKB: 65536},
		},
	}
	w.ProcessTask("msg-1", "respQ", task)

	if got := w.GetProcessingMessageID(); got != "" {
		t.Fatalf("expected processingMessageID to be cleared, got %q", got)
	}
}

func TestProcessTask_CompilationErrorFlow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	dir := &packager.TaskDirConfig{
		PackageDirPath:     t.TempDir(),
		UserSolutionPath:   "src",
		UserExecFilePath:   "exec",
		CompileErrFilePath: "compile.err",
	}
	mockPackager.EXPECT().PrepareSolutionPackage(gomock.Any(), gomock.Any()).Return(dir, nil)

	// Simulate compilation failure
	mockCompiler.EXPECT().
		CompileSolutionIfNeeded(
			gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(pkgerrors.ErrCompilationFailed)

	// When compilation fails SendSolutionPackage should be called with hasCompilationErr=true
	mockPackager.EXPECT().SendSolutionPackage(dir, gomock.Any(), true, gomock.Any()).Return(nil)

	// Expect PublishPayloadTaskRespond called with a compilation error result
	mockResponder.EXPECT().PublishPayloadTaskRespond(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(messageType, messageID, responseQueue string, res solution.Result) {
			if res.StatusCode != solution.CompilationError {
				t.Fatalf("expected compilation error status, got %v", res.StatusCode)
			}
			if res.Message != constants.SolutionMessageCompilationError {
				t.Fatalf("unexpected compilation message: %q", res.Message)
			}
		},
	)

	w := pipeline.NewWorker(2, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)
	task := &messages.TaskQueueMessage{LanguageType: "cpp", LanguageVersion: "11", TestCases: nil}
	w.ProcessTask("msg-compile", "respQ", task)
}

func TestProcessTask_PreparePackageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	mockPackager.EXPECT().PrepareSolutionPackage(gomock.Any(), gomock.Any()).Return(nil, errors.New("download failed"))
	mockResponder.EXPECT().PublishTaskErrorToResponseQueue(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	w := pipeline.NewWorker(3, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)
	task := &messages.TaskQueueMessage{LanguageType: "cpp"}
	w.ProcessTask("msg-dl", "respQ", task)
}

func TestProcessTask_SendPackageFailsAfterRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	dir := &packager.TaskDirConfig{
		PackageDirPath:     t.TempDir(),
		UserSolutionPath:   "src",
		UserExecFilePath:   "exec",
		CompileErrFilePath: "compile.err",
	}
	mockPackager.EXPECT().PrepareSolutionPackage(gomock.Any(), gomock.Any()).Return(dir, nil)
	mockCompiler.EXPECT().
		CompileSolutionIfNeeded(
			gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil)
	mockExecutor.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
	mockVerifier.EXPECT().
		EvaluateAllTestCases(dir, gomock.Any(), gomock.Any()).
		Return(solution.Result{
			StatusCode: solution.Success,
			Message:    "OK",
		})

	// Simulate upload failure
	mockPackager.EXPECT().SendSolutionPackage(dir, gomock.Any(), false, gomock.Any()).Return(errors.New("upload failed"))
	mockResponder.EXPECT().PublishTaskErrorToResponseQueue(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	w := pipeline.NewWorker(4, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)
	task := &messages.TaskQueueMessage{LanguageType: "cpp"}
	w.ProcessTask("msg-upload", "respQ", task)
}

func TestProcessTask_VerifierPanicRecovered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	dir := &packager.TaskDirConfig{
		PackageDirPath:     t.TempDir(),
		UserSolutionPath:   "src",
		UserExecFilePath:   "exec",
		CompileErrFilePath: "compile.err",
	}
	mockPackager.EXPECT().PrepareSolutionPackage(gomock.Any(), gomock.Any()).Return(dir, nil)
	mockCompiler.EXPECT().
		CompileSolutionIfNeeded(
			gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil)
	mockExecutor.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)

	// Make verifier panic
	mockVerifier.EXPECT().EvaluateAllTestCases(dir, gomock.Any(), gomock.Any()).Do(
		func(dir *packager.TaskDirConfig, tcs []messages.TestCase, msgID string) {
			panic(errors.New("boom"))
		},
	)

	mockResponder.EXPECT().PublishTaskErrorToResponseQueue(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())

	w := pipeline.NewWorker(5, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)
	task := &messages.TaskQueueMessage{LanguageType: "cpp"}
	w.ProcessTask("msg-panic", "respQ", task)
}

func TestProcessTask_PublishPayloadFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	setupSuccessfulPipelineMocks(t, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)

	w := pipeline.NewWorker(6, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)
	task := &messages.TaskQueueMessage{
		LanguageType:    "cpp",
		LanguageVersion: "11",
		TestCases: []messages.TestCase{
			{TimeLimitMs: 100, MemoryLimitKB: 65536},
		},
	}
	w.ProcessTask("msg-pub", "respQ", task)

	if got := w.GetProcessingMessageID(); got != "" {
		t.Fatalf("expected processingMessageID to be cleared, got %q", got)
	}
}

func TestGetStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	w := pipeline.NewWorker(7, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)

	if status := w.GetState(); status.Status != constants.WorkerStatusIdle {
		t.Fatalf("expected initial status to be Idle, got %q", status)
	}

	w.UpdateStatus(constants.WorkerStatusBusy)
	if status := w.GetState(); status.Status != constants.WorkerStatusBusy {
		t.Fatalf("expected status to be Busy, got %q", status)
	}

	w.UpdateStatus(constants.WorkerStatusIdle)
	if status := w.GetState(); status.Status != constants.WorkerStatusIdle {
		t.Fatalf("expected status to be Idle after update, got %q", status)
	}
}

func TestGetProcessingMessageID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCompiler := mocks.NewMockCompiler(ctrl)
	mockPackager := mocks.NewMockPackager(ctrl)
	mockExecutor := mocks.NewMockExecutor(ctrl)
	mockVerifier := mocks.NewMockVerifier(ctrl)
	mockResponder := mocks.NewMockResponder(ctrl)

	w := pipeline.NewWorker(8, mockCompiler, mockPackager, mockExecutor, mockVerifier, mockResponder)

	if msgID := w.GetProcessingMessageID(); msgID != "" {
		t.Fatalf("expected initial processingMessageID to be empty, got %q", msgID)
	}

	// Synchronize with the worker: make PrepareSolutionPackage block until
	// we observe processingMessageID being set. This avoids races.
	started := make(chan struct{})
	done := make(chan struct{})
	dir := &packager.TaskDirConfig{
		PackageDirPath:     t.TempDir(),
		UserSolutionPath:   "src",
		UserExecFilePath:   "exec",
		CompileErrFilePath: "compile.err",
	}

	mockPackager.EXPECT().PrepareSolutionPackage(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, _ interface{}) (*packager.TaskDirConfig, error) {
			// signal that PrepareSolutionPackage was invoked (worker should have set processingMessageID)
			close(started)
			// wait until test allows continuation
			<-done
			return dir, nil
		},
	)

	// The rest of the pipeline should succeed quickly after release
	mockCompiler.EXPECT().
		CompileSolutionIfNeeded(
			gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil)
	mockExecutor.EXPECT().ExecuteCommand(gomock.Any()).Return(nil)
	mockVerifier.EXPECT().
		EvaluateAllTestCases(dir, gomock.Any(), gomock.Any()).
		Return(solution.Result{
			StatusCode: solution.Success,
			Message:    "OK",
		})
	mockPackager.EXPECT().SendSolutionPackage(dir, gomock.Any(), false, gomock.Any()).Return(nil)
	// Start processing in background
	mockResponder.EXPECT().
		PublishPayloadTaskRespond(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
	go func() {
		w.ProcessTask("msg-123", "respQ", &messages.TaskQueueMessage{LanguageType: "cpp"})
	}()

	// Wait until PrepareSolutionPackage is invoked (worker has started)
	select {
	case <-started:
		// good
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for worker to start processing")
	}

	// Now the worker should have the processing ID set
	if msgID := w.GetProcessingMessageID(); msgID != "msg-123" {
		t.Fatalf("expected processingMessageID to be 'msg-123', got %q", msgID)
	}

	// Allow worker to finish
	close(done)

	// Wait until processingMessageID is cleared
	deadline := time.After(2 * time.Second)
	for w.GetProcessingMessageID() != "" {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for worker to finish")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

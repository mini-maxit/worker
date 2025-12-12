package packager_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/tests/mocks"
	gomock "go.uber.org/mock/gomock"
)

func TestPrepareSolutionPackage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockStorage(ctrl)
	// prepare message
	submission := messages.FileLocation{Bucket: "sub", Path: "solutions/1/main.cpp"}
	tc := messages.TestCase{
		Order:          1,
		InputFile:      messages.FileLocation{Bucket: "inputs", Path: "inputs/1/in.txt"},
		ExpectedOutput: messages.FileLocation{Bucket: "outputs", Path: "outputs/1/out.txt"},
		StdOutResult:   messages.FileLocation{Bucket: "results", Path: "results/1/out.result"},
		StdErrResult:   messages.FileLocation{Bucket: "results", Path: "results/1/err.result"},
		DiffResult:     messages.FileLocation{Bucket: "results", Path: "results/1/diff.result"},
	}
	msg := &messages.TaskQueueMessage{SubmissionFile: submission, TestCases: []messages.TestCase{tc}}
	msgID := "pkg-test-1"

	// expect DownloadFile for submission and test case files; destination path can be any
	mockStorage.EXPECT().DownloadFile(submission, gomock.Any()).Return("/tmp/dest-sub", nil)
	mockStorage.EXPECT().DownloadFile(tc.InputFile, gomock.Any()).Return("/tmp/dest-in", nil)
	mockStorage.EXPECT().DownloadFile(tc.ExpectedOutput, gomock.Any()).Return("/tmp/dest-out", nil)

	p := packager.NewPackager(mockStorage)

	cfg, err := p.PrepareSolutionPackage(msg, msgID)
	if err != nil {
		t.Fatalf("PrepareSolutionPackage failed: %v", err)
	}

	// verify directories and files exist
	if _, err := os.Stat(cfg.PackageDirPath); err != nil {
		t.Fatalf("expected package dir to exist: %v", err)
	}
	if _, err := os.Stat(cfg.InputDirPath); err != nil {
		t.Fatalf("expected input dir to exist: %v", err)
	}
	if _, err := os.Stat(cfg.OutputDirPath); err != nil {
		t.Fatalf("expected output dir to exist: %v", err)
	}
	if _, err := os.Stat(cfg.UserOutputDirPath); err != nil {
		t.Fatalf("expected user output dir to exist: %v", err)
	}
	// compile err file should exist
	if fi, err := os.Stat(cfg.CompileErrFilePath); err != nil {
		t.Fatalf("expected compile.err to exist: %v", err)
	} else if fi.Size() != 0 {
		t.Fatalf("expected compile.err to be empty, size=%d", fi.Size())
	}

	// user result files should be created using task-provided names
	userOutPath := filepath.Join(cfg.UserOutputDirPath, filepath.Base(tc.StdOutResult.Path))
	if _, err := os.Stat(userOutPath); err != nil {
		t.Fatalf("expected user stdout file to exist: %v", err)
	}

	userErrPath := filepath.Join(cfg.UserErrorDirPath, filepath.Base(tc.StdErrResult.Path))
	if _, err := os.Stat(userErrPath); err != nil {
		t.Fatalf("expected user stderr file to exist: %v", err)
	}

	userDiffPath := filepath.Join(cfg.UserDiffDirPath, filepath.Base(tc.DiffResult.Path))
	if _, err := os.Stat(userDiffPath); err != nil {
		t.Fatalf("expected user diff file to exist: %v", err)
	}

	userResPath := filepath.Join(
		cfg.UserExecResultDirPath,
		fmt.Sprintf("%d.%s", tc.Order, constants.ExecutionResultFileExt),
	)
	if _, err := os.Stat(userResPath); err != nil {
		t.Fatalf("expected user exec result file to exist: %v", err)
	}

	// cleanup created temp directory under /tmp
	_ = os.RemoveAll(cfg.PackageDirPath)
}

func TestPrepareSolutionPackage_NoStorage(t *testing.T) {
	p := packager.NewPackager(nil)
	_, err := p.PrepareSolutionPackage(&messages.TaskQueueMessage{}, "id-no-storage")
	if err == nil {
		t.Fatalf("expected error when storage is nil")
	}
	if !strings.Contains(err.Error(), "storage service is not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendSolutionPackage_WithCompilationError_Uploads(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockStorage(ctrl)

	dir := t.TempDir()
	compErrPath := filepath.Join(dir, "compile.err")
	if err := os.WriteFile(compErrPath, []byte("compile error"), 0o644); err != nil {
		t.Fatalf("failed to write compile.err: %v", err)
	}

	// test case describing where to upload
	tc := messages.TestCase{StdErrResult: messages.FileLocation{Bucket: "res-bucket", Path: "some/path/compile.err"}}

	// expect UploadFile with objPath equal to parent dir of Path
	mockStorage.EXPECT().UploadFile(compErrPath, "res-bucket", "some/path").Return(nil)

	p := packager.NewPackager(mockStorage)

	cfg := &packager.TaskDirConfig{CompileErrFilePath: compErrPath}
	if err := p.SendSolutionPackage(cfg, []messages.TestCase{tc}, true, "msg-id"); err != nil {
		t.Fatalf("SendSolutionPackage returned error: %v", err)
	}
}

func TestSendSolutionPackage_NoCompilation_UploadsNonEmptyFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mocks.NewMockStorage(ctrl)

	dir := t.TempDir()
	userOutDir := filepath.Join(dir, "userOut")
	userErrDir := filepath.Join(dir, "userErr")
	userDiffDir := filepath.Join(dir, "userDiff")
	if err := os.MkdirAll(userOutDir, 0o755); err != nil {
		t.Fatalf("failed to create userOutDir: %v", err)
	}
	if err := os.MkdirAll(userErrDir, 0o755); err != nil {
		t.Fatalf("failed to create userErrDir: %v", err)
	}
	if err := os.MkdirAll(userDiffDir, 0o755); err != nil {
		t.Fatalf("failed to create userDiffDir: %v", err)
	}

	// prepare test case with paths containing slashes
	tc := messages.TestCase{
		StdOutResult: messages.FileLocation{Bucket: "b", Path: "outputs/task1/out.txt"},
		StdErrResult: messages.FileLocation{Bucket: "b", Path: "errors/task1/err.txt"},
		DiffResult:   messages.FileLocation{Bucket: "b", Path: "diffs/task1/diff.txt"},
	}

	// create user files with content
	userOutPath := filepath.Join(userOutDir, filepath.Base(tc.StdOutResult.Path))
	userErrPath := filepath.Join(userErrDir, filepath.Base(tc.StdErrResult.Path))
	userDiffPath := filepath.Join(userDiffDir, filepath.Base(tc.DiffResult.Path))
	if err := os.WriteFile(userOutPath, []byte("out"), 0o644); err != nil {
		t.Fatalf("failed to write userOutPath: %v", err)
	}
	if err := os.WriteFile(userErrPath, []byte("err"), 0o644); err != nil {
		t.Fatalf("failed to write userErrPath: %v", err)
	}
	if err := os.WriteFile(userDiffPath, []byte("diff"), 0o644); err != nil {
		t.Fatalf("failed to write userDiffPath: %v", err)
	}

	// expect UploadFile for each non-empty file, with objPath equal to parent dir of Path
	mockStorage.EXPECT().UploadFile(userOutPath, "b", "outputs/task1").Return(nil)
	mockStorage.EXPECT().UploadFile(userErrPath, "b", "errors/task1").Return(nil)
	mockStorage.EXPECT().UploadFile(userDiffPath, "b", "diffs/task1").Return(nil)

	p := packager.NewPackager(mockStorage)

	cfg := &packager.TaskDirConfig{
		UserOutputDirPath: userOutDir,
		UserErrorDirPath:  userErrDir,
		UserDiffDirPath:   userDiffDir,
	}

	if err := p.SendSolutionPackage(cfg, []messages.TestCase{tc}, false, "msg-id"); err != nil {
		t.Fatalf("SendSolutionPackage returned error: %v", err)
	}
}

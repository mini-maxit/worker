package executor_test

import (
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	exec "github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	mocks "github.com/mini-maxit/worker/tests/mocks"
	"go.uber.org/mock/gomock"
)

func makeDirConfig() *packager.TaskDirConfig {
	return &packager.TaskDirConfig{
		TmpDirPath:            "/tmp",
		PackageDirPath:        "/tmp/pkg",
		InputDirPath:          "/tmp/pkg/input",
		UserOutputDirPath:     "/tmp/pkg/userout",
		UserErrorDirPath:      "/tmp/pkg/usrerr",
		UserExecFilePath:      "/tmp/pkg/solution",
		UserExecResultDirPath: "/tmp/pkg/res",
	}
}

func TestSanitizeContainerName(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"abc123", "submission-abc123"},
		{"A.B-C_D", "submission-A.B-C_D"},
		{"123", "submission-123"},
		{"000123", "submission-000123"},
		{"42", "submission-42"},
		{"9", "submission-9"},
		{"", "submission-untitled"},
		{"bad name!", "submission-bad-name-"},
		{"..weird..name..", "submission-..weird..name.."},
	}

	for _, c := range cases {
		got := exec.SanitizeContainerName(c.in)
		if got != c.out {
			t.Fatalf("sanitizeContainerName(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

// setupMockExpectations sets up common mock expectations for testing exclude functionality.
func setupMockExpectations(
	mockDocker *mocks.MockDockerClient,
	containerID string,
	capturedExcludes *[]string,
	statusCode int64,
) {
	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(containerID, nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), containerID, gomock.Any(), gomock.Any(), gomock.Any(),
		).DoAndReturn(func(
			ctx interface{},
			cID, srcPath, dstPath string,
			excludes []string,
		) error {
			*capturedExcludes = excludes
			return nil
		}).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), containerID).Return(nil).Times(1),
		mockDocker.EXPECT().WaitContainer(
			gomock.Any(), containerID, gomock.Any(),
		).Return(statusCode, nil).Times(1),
		mockDocker.EXPECT().CopyFromContainerFiltered(
			gomock.Any(), containerID, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), containerID).Times(1),
	)
}

func makeTestCases() []messages.TestCase {
	return []messages.TestCase{{
		Order:         1,
		InputFile:     messages.FileLocation{Path: "in.txt"},
		StdOutResult:  messages.FileLocation{Path: "out.txt"},
		StdErrResult:  messages.FileLocation{Path: "err.txt"},
		TimeLimitMs:   100,
		MemoryLimitKB: 256,
	}}
}

func TestExecuteCommand_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	// Prepare channels for ContainerWait returning successful exit code
	statusCh := make(chan container.WaitResponse, 1)
	statusCh <- container.WaitResponse{StatusCode: 0}

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid123", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid123", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), "cid123").Return(nil).Times(1),
		mockDocker.EXPECT().WaitContainer(
			gomock.Any(), "cid123", gomock.Any(),
		).Return(int64(0), nil).Times(1),
		mockDocker.EXPECT().CopyFromContainerFiltered(
			gomock.Any(), "cid123", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid123").Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg1",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	if err := ex.ExecuteCommand(cfg); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestExecuteCommand_ContainerNonZeroExit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	statusCh := make(chan container.WaitResponse, 1)
	statusCh <- container.WaitResponse{StatusCode: 2}

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid-nonzero", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid-nonzero", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), "cid-nonzero").Return(nil).Times(1),
		mockDocker.EXPECT().WaitContainer(
			gomock.Any(), "cid-nonzero", gomock.Any(),
		).Return(int64(1), nil).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid-nonzero").Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg2",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected error due to non-zero exit, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrContainerFailed) {
		t.Fatalf("expected ErrContainerFailed, got: %v", err)
	}
}

func TestExecuteCommand_EnsureImageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(errors.New("ensure-failed")).Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg3",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	if err := ex.ExecuteCommand(cfg); err == nil {
		t.Fatalf("expected ensure image error, got nil")
	}
}

func TestExecuteCommand_CreateContainerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("", errors.New("create-failed")).Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg4",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	if err := ex.ExecuteCommand(cfg); err == nil {
		t.Fatalf("expected create container error, got nil")
	}
}

func TestExecuteCommand_CopyToContainerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid-copyto-fail", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid-copyto-fail", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(errors.New("copy-to-failed")).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid-copyto-fail").Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg-copyto-fail",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected error from CopyToContainerFiltered, got nil")
	}
}

func TestExecuteCommand_StartContainerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid-start-fail", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid-start-fail", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), "cid-start-fail").Return(errors.New("start-failed")).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid-start-fail").Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg-start-fail",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected error from StartContainer, got nil")
	}
}

func TestExecuteCommand_CopyFromContainerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid-copyfrom-fail", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid-copyfrom-fail", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), "cid-copyfrom-fail").Return(nil).Times(1),
		mockDocker.EXPECT().WaitContainer(
			gomock.Any(), "cid-copyfrom-fail", gomock.Any(),
		).Return(int64(0), nil).Times(1),
		mockDocker.EXPECT().CopyFromContainerFiltered(
			gomock.Any(), "cid-copyfrom-fail", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(errors.New("copy-from-failed")).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid-copyfrom-fail").Times(1),
	)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg-copyfrom-fail",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected error from CopyFromContainerFiltered, got nil")
	}
}

func TestExecuteCommand_GetDockerImageFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg5",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.LanguageType(0), // invalid language type
		LanguageVersion: "",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected error from GetDockerImage, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrInvalidLanguageType) {
		t.Fatalf("expected ErrInvalidLanguageType, got: %v", err)
	}
}

func TestWaitForContainer_ErrFromErrCh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	errCh := make(chan error, 1)
	errCh <- errors.New("wait-err")

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), "cid").Return(nil).Times(1),
		mockDocker.EXPECT().WaitContainer(
			gomock.Any(), "cid", gomock.Any(),
		).Return(int64(-1), errors.New("wait-err")).Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid").Times(1),
	)

	ex := exec.NewExecutor(mockDocker, 5)

	cfg := exec.CommandConfig{
		MessageID:       "msg-wait-err",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected error from errCh, got nil")
	}
	if err.Error() != "wait-err" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForContainer_TimeoutCallsKill(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	gomock.InOrder(
		mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil).Times(1),
		mockDocker.EXPECT().CreateContainer(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return("cid-timeout", nil).Times(1),
		mockDocker.EXPECT().CopyToContainerFiltered(
			gomock.Any(), "cid-timeout", gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil).Times(1),
		mockDocker.EXPECT().StartContainer(gomock.Any(), "cid-timeout").Return(nil).Times(1),
		mockDocker.EXPECT().WaitContainer(
			gomock.Any(), "cid-timeout", gomock.Any(),
		).Return(int64(-1), pkgerrors.ErrContainerTimeout).Times(1),
		mockDocker.EXPECT().ContainerKill(gomock.Any(), "cid-timeout", "SIGKILL").Times(1),
		mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid-timeout").Times(1),
	)

	ex := exec.NewExecutor(mockDocker, 1)

	cfg := exec.CommandConfig{
		MessageID:       "msg-timeout",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !errors.Is(err, pkgerrors.ErrContainerTimeout) {
		t.Fatalf("expected ErrContainerTimeout, got: %v", err)
	}
}

func TestExecuteCommand_ExcludesExpectedOutputs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	// Capture the excludes parameter to verify it contains OutputDirName.
	var capturedExcludes []string

	setupMockExpectations(mockDocker, "cid-excludes", &capturedExcludes, 0)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg-excludes",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Verify that OutputDirName is in the excludes list
	found := false
	for _, exclude := range capturedExcludes {
		if exclude == constants.OutputDirName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected OutputDirName (%s) to be in excludes list, got: %v", constants.OutputDirName, capturedExcludes)
	}
}

func TestExecuteCommand_OnlyExpectedOutputsExcluded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDocker := mocks.NewMockDockerClient(ctrl)

	// Capture the excludes parameter to verify it only contains OutputDirName.
	var capturedExcludes []string

	setupMockExpectations(mockDocker, "cid-only-outputs", &capturedExcludes, 0)

	ex := exec.NewExecutor(mockDocker)

	cfg := exec.CommandConfig{
		MessageID:       "msg-only-outputs",
		DirConfig:       makeDirConfig(),
		LanguageType:    languages.CPP,
		LanguageVersion: "17",
		TestCases:       makeTestCases(),
	}

	err := ex.ExecuteCommand(cfg)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Verify that only OutputDirName is in the excludes list
	if len(capturedExcludes) != 1 {
		t.Fatalf("expected exactly 1 exclude, got %d: %v", len(capturedExcludes), capturedExcludes)
	}
	if capturedExcludes[0] != constants.OutputDirName {
		t.Fatalf("expected exclude to be OutputDirName (%s), got: %s", constants.OutputDirName, capturedExcludes[0])
	}
}

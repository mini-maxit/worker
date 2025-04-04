package tests

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mini-maxit/worker/internal/services"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
)

const (
	mockFilesDir        = "./mock_files/"
	mockTaskFilesDir    = mockFilesDir + "Task"
	mockUserSolutionDir = mockFilesDir + "solutions/"
	MockTmpDir          = mockFilesDir + "tmp"
)

type TestType int

const (
	CPPSuccess TestType = iota + 1
	CPPFailedTimeLimitExceeded
	CPPCompilationError
	CPPTestCaseFailed
	Handshake
	LongTaskMessage
	Status
)

var testTypeSolutionMap = map[TestType]string{
	CPPSuccess:                 "CPPSuccessSolution.cpp",
	CPPFailedTimeLimitExceeded: "CPPFailedTimeLimitExceededSolution.cpp",
	CPPCompilationError:        "CPPCompilationErrorSolution.cpp",
	CPPTestCaseFailed:          "CPPTestCaseFailedSolution.cpp",
	LongTaskMessage:            "CPPFailedTimeLimitExceededSolution.cpp",
}

type MockFileService struct {
	t *testing.T
}

func NewMockFileService(t *testing.T) services.FileService {
	return &MockFileService{
		t: t,
	}
}

func (mfs *MockFileService) HandleTaskPackage(taskID, userID, submissionNumber int64) (*services.TaskDirConfig, error) {
	if _, err := os.Stat(MockTmpDir); os.IsNotExist(err) {
		return &services.TaskDirConfig{}, fmt.Errorf("temporary directory does not exist: %s", MockTmpDir)
	}

	dirName := fmt.Sprintf("Task_%d_%d_%d", taskID, userID, submissionNumber)
	dirPath := filepath.Join(MockTmpDir, dirName)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return &services.TaskDirConfig{}, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Cleanup on failure
	defer func() {
		if err != nil {
			os.RemoveAll(dirPath)
		}
	}()

	if _, err := os.Stat(mockTaskFilesDir); os.IsNotExist(err) {
		return &services.TaskDirConfig{}, fmt.Errorf("task files directory does not exist: %s", mockTaskFilesDir)
	}

	if err := copyDir(mockTaskFilesDir, dirPath); err != nil {
		return &services.TaskDirConfig{}, fmt.Errorf("failed to copy task files: %w", err)
	}

	solutionFile, ok := testTypeSolutionMap[TestType(submissionNumber)]
	if !ok {
		return &services.TaskDirConfig{}, fmt.Errorf("invalid submission number: %d", submissionNumber)
	}
	userSolution := filepath.Join(mockUserSolutionDir, solutionFile)
	solutionName := "solution" + filepath.Ext(userSolution)
	destSolution := filepath.Join(dirPath, solutionName)

	if _, err := os.Stat(userSolution); os.IsNotExist(err) {
		return &services.TaskDirConfig{}, fmt.Errorf("user solution file does not exist: %s", userSolution)
	}

	if err := utils.CopyFile(userSolution, destSolution); err != nil {
		return &services.TaskDirConfig{}, fmt.Errorf("failed to copy user solution file: %w", err)
	}

	return &services.TaskDirConfig{
		TaskFilesDirPath:    dirPath,
		UserSolutionDirPath: destSolution,
	}, nil
}

func copyDir(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %w", path, err)
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		targetPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}
		} else {
			if err := utils.CopyFile(path, targetPath); err != nil {
				return fmt.Errorf("failed to copy file %s to %s: %w", path, targetPath, err)
			}
		}

		return nil
	})
}

func (mfs *MockFileService) UnconpressPackage(_ string) (*services.TaskDirConfig, error) {
	return &services.TaskDirConfig{}, errors.New("UncompressPackage not implemented")
}

func (mfs *MockFileService) StoreSolutionResult(_ solution.Result, _ string, _, _, _ int64) error {
	return nil
}

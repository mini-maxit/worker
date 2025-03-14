package tests

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mini-maxit/worker/internal/services"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
)

const (
	mockFilesDir        = "./mock_files/"
	mockTaskFilesDir    = mockFilesDir + "Task"
	mockUserSolutionDir = mockFilesDir + "solutions/"
	mockTmpDir          = mockFilesDir + "tmp"
)

type MockFileService struct {
	t *testing.T
}

func NewMockFileService(t *testing.T) services.FileService {
	return &MockFileService{
		t: t,
	}
}

func (mfs *MockFileService) HandleTaskPackage(taskId, userId, submissionNumber int64) (services.TaskDirConfig, error) {
	if _, err := os.Stat(mockTmpDir); os.IsNotExist(err) {
		return services.TaskDirConfig{}, fmt.Errorf("temporary directory does not exist: %s", mockTmpDir)
	}

	dirName := fmt.Sprintf("Task_%d_%d_%d", taskId, userId, submissionNumber)
	dirPath := filepath.Join(mockTmpDir, dirName)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return services.TaskDirConfig{}, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Cleanup on failure
	defer func() {
		if err != nil {
			os.RemoveAll(dirPath)
		}
	}()

	if _, err := os.Stat(mockTaskFilesDir); os.IsNotExist(err) {
		return services.TaskDirConfig{}, fmt.Errorf("task files directory does not exist: %s", mockTaskFilesDir)
	}

	if err := copyDir(mockTaskFilesDir, dirPath); err != nil {
		return services.TaskDirConfig{}, fmt.Errorf("failed to copy task files: %w", err)
	}

	userSolution := filepath.Join(mockUserSolutionDir, strconv.FormatInt(submissionNumber, 10)+".cpp")
	destSolution := filepath.Join(dirPath, "solution.cpp")

	if _, err := os.Stat(userSolution); os.IsNotExist(err) {
		return services.TaskDirConfig{}, fmt.Errorf("user solution file does not exist: %s", userSolution)
	}

	if err := utils.CopyFile(userSolution, destSolution); err != nil {
		return services.TaskDirConfig{}, fmt.Errorf("failed to copy user solution file: %w", err)
	}

	return services.TaskDirConfig{
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

func (mfs *MockFileService) UnconpressPackage(zipFilePath string) (services.TaskDirConfig, error) {
	return services.TaskDirConfig{}, errors.New("UncompressPackage not implemented")
}

func (mfs *MockFileService) StoreSolutionResult(solutionResult solution.SolutionResult, taskFilesDirPath string, userId, taskId, submissionNumber int64) error {
	return nil
}

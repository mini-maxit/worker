package tests

import (
	"strconv"

	"github.com/mini-maxit/worker/internal/services"
	"github.com/mini-maxit/worker/solution"
)

const mock_files_dir = "./mock_files/"
const mock_task_files_dir = mock_files_dir + "Task/"
const mock_user_solution_dir = mock_files_dir + "solutions/"

type MockFileService struct {
}

func NewMockFileService() services.FileService {
	return &MockFileService{}
}

func (mfs *MockFileService) HandleTaskPackage(taskId, userId, submissionNumber int64) (services.TaskDirConfig, error) {
	dir := services.TaskDirConfig{
		TaskFilesDirPath:    mock_task_files_dir,
		UserSolutionDirPath: mock_user_solution_dir + strconv.FormatInt(submissionNumber, 10),
	}
	return dir, nil
}

func (mfs *MockFileService) UnconpressPackage(zipFilePath string) (services.TaskDirConfig, error) {
	panic("implement me")
}

func (mfs *MockFileService) StoreSolutionResult(solutionResult solution.SolutionResult, taskFilesDirPath string, userId, taskId, submissionNumber int64) error {
	return nil
}

package worker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type TaskForRunner struct {
	TaskDir          string
	TempDir          string
	LanguageType     solution.LanguageType
	LanguageVersion  string
	SolutionFileName string
	InputDirName     string
	OutputDirName    string
	TimeLimits       []int
	MemoryLimits     []int
}

type DirConfig struct {
	TempDir string
	TaskDir string
}

type SolutionData struct {
	taskId           int64
	userId           int64
	submissionNumber int64
	logger           *zap.SugaredLogger
}

// GetDataForSolutionRunner retrieves the data needed to run the solution
func (sd *SolutionData) getDataForSolutionRunner(fileStorageUrl string) (TaskForRunner, error) {
	var task TaskForRunner

	sd.logger.Infof("Getting solution package [TaskID: %d, UserID: %d, SubmissionNumber: %d]", sd.taskId, sd.userId, sd.submissionNumber)
	requestUrl := fmt.Sprintf("%s/getSolutionPackage?taskID=%d&userID=%d&submissionNumber=%d", fileStorageUrl, sd.taskId, sd.userId, sd.submissionNumber)
	response, err := http.Get(requestUrl)
	if err != nil {
		sd.logger.Errorf("Failed to get solution package. %s", err)
		return TaskForRunner{}, err
	}

	if response.StatusCode != 200 {
		sd.logger.Errorf("Failed to get solution package. %s", response.Status)
		bodyBytes, _ := io.ReadAll(response.Body)
		return TaskForRunner{}, errors.New(string(bodyBytes))
	}

	id := uuid.New()
	filePath := fmt.Sprintf("/app/%s.tar.gz", id.String())
	file, err := os.Create(filePath)
	if err != nil {
		sd.logger.Errorf("Failed to create file. %s", err)
		return TaskForRunner{}, err
	}

	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		sd.logger.Errorf("Failed to copy file. %s", err)
		return TaskForRunner{}, err
	}

	dirConfig, err := sd.handlePackage(filePath)
	if err != nil {
		return TaskForRunner{}, err
	}

	task.TempDir = dirConfig.TempDir
	task.TaskDir = dirConfig.TaskDir

	file.Close()

	return task, nil
}

// unzips the package and returns the directories configuration
func (sd *SolutionData) handlePackage(zipFilePath string) (DirConfig, error) {
	sd.logger.Infof("Unzipping solution package [Path: %s]", zipFilePath)

	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		sd.logger.Errorf("Failed to create temp directory. %s", err)
		return DirConfig{}, err
	}

	err = os.Rename(zipFilePath, path+"/file.tar.gz")
	if err != nil {
		sd.logger.Errorf("Failed to rename file. %s", err)
		errRemove := utils.RemoveIO(path, true, true)
		if errRemove != nil {
			sd.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		}
		return DirConfig{}, err
	}

	err = utils.ExtractTarGz(path+"/file.tar.gz", path)
	if err != nil {
		sd.logger.Errorf("Failed to extract file. %s", err)
		errRemove := utils.RemoveIO(path, true, true)
		if errRemove != nil {
			sd.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		}
		return DirConfig{}, err
	}

	errRemove := utils.RemoveIO(path+"/file.tar.gz", false, false)
	if errRemove != nil {
		sd.logger.Errorf("Failed to remove file. %s", errRemove)
	}

	dirConfig := DirConfig{
		TempDir: path,
		TaskDir: path + "/Task",
	}

	return dirConfig, nil
}

func NewSolutionData(taskId, userId, submissionNumber int64) *SolutionData {
	logger := logger.NewNamedLogger("solutionData")

	return &SolutionData{
		taskId:           taskId,
		userId:           userId,
		submissionNumber: submissionNumber,
		logger:           logger,
	}
}

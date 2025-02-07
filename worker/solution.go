package worker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/mini-maxit/worker/solution"
	"github.com/mini-maxit/worker/utils"
)
type TaskForRunner struct {
	TaskDir            string
	TempDir            string
	LanguageType       solution.LanguageType
	LanguageVersion    string
	SolutionFileName   string
	InputDirName        string
	OutputDirName 		string
	TimeLimits 	       []int
	MemoryLimits 	   []int
}

type DirConfig struct {
	TempDir            string
    TaskDir            string
}


// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(taskId, userId, submissionNumber int64, fileStorageUrl string) (TaskForRunner, error) {
	var task TaskForRunner

	requestUrl := fmt.Sprintf("%s/getSolutionPackage?taskID=%d&userID=%d&submissionNumber=%d", fileStorageUrl, taskId, userId, submissionNumber)
	response, err := http.Get(requestUrl)
	if err != nil {
		return TaskForRunner{}, err
	}

	if(response.StatusCode != 200) {
		bodyBytes, _ := io.ReadAll(response.Body)
		return TaskForRunner{}, errors.New(string(bodyBytes))
	}

	id := uuid.New()
	filePath := fmt.Sprintf("/app/%s.tar.gz", id.String())
	file, err := os.Create(filePath)
	if err != nil {
		return TaskForRunner{}, err
	}

	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return TaskForRunner{}, err
	}

	dirConfig, err := handlePackage(filePath)
	if err != nil {
		return TaskForRunner{}, err
	}

	task.TempDir = dirConfig.TempDir
	task.TaskDir = dirConfig.TaskDir

	file.Close()

	return task, nil
}

// unzips the package and returns the directories configuration
func handlePackage(zipFilePath string) (DirConfig, error) {

	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		return DirConfig{}, err
	}

	err = os.Rename(zipFilePath, path  + "/file.tar.gz")
	if err != nil {
		utils.RemoveIO(path, true, true)
		return DirConfig{}, err
	}

	err = utils.ExtractTarGz(path + "/file.tar.gz", path)
	if err != nil {
		utils.RemoveIO(path, true, true)
		return DirConfig{}, err
	}

	utils.RemoveIO(path + "/file.tar.gz", false, false)

	dirConfig := DirConfig{
		TempDir:            path,
		TaskDir:            path + "/Task",
	}

	return dirConfig, nil
}

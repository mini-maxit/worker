package services

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"github.com/mini-maxit/worker/internal/constants"
	cutomErrors "github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type FileService interface {
	HandleTaskPackage(taskId, userId, submissionNumber int64) (TaskDirConfig, error)
	UnconpressPackage(zipFilePath string) (TaskDirConfig, error)
	StoreSolutionResult(solutionResult solution.SolutionResult, TaskFilesDirPath string, userId, taskId, submissionNumber int64) error
}

type fileService struct {
	fileStorageUrl string
	logger         *zap.SugaredLogger
}

type TaskDirConfig struct {
	TaskFilesDirPath    string
	UserSolutionDirPath string
}

func NewFilesService(fileServiceURL string) FileService {
	logger := logger.NewNamedLogger("fileService")
	return &fileService{
		fileStorageUrl: fileServiceURL,
		logger:         logger,
	}
}

func (fs *fileService) HandleTaskPackage(taskId, userId, submissionNumber int64) (TaskDirConfig, error) {
	fs.logger.Info("Handling task package")
	requestUrl := fmt.Sprintf("%s/getSolutionPackage?taskID=%d&userID=%d&submissionNumber=%d", fs.fileStorageUrl, taskId, userId, submissionNumber)
	response, err := http.Get(requestUrl)
	if err != nil {
		fs.logger.Errorf("Failed to get solution package. %s", err)
		return TaskDirConfig{}, err
	}

	if response.StatusCode != 200 {
		fs.logger.Errorf("Failed to get solution package. %s", response.Status)
		bodyBytes, _ := io.ReadAll(response.Body)
		return TaskDirConfig{}, errors.New(string(bodyBytes))
	}

	id := uuid.New()
	filePath := fmt.Sprintf("/app/%s.tar.gz", id.String())
	file, err := os.Create(filePath)
	if err != nil {
		fs.logger.Errorf("Failed to create file. %s", err)
		return TaskDirConfig{}, err
	}

	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		fs.logger.Errorf("Failed to copy file. %s", err)
		return TaskDirConfig{}, err
	}

	taskDirConfig, err := fs.UnconpressPackage(filePath)
	if err != nil {
		return TaskDirConfig{}, err
	}

	file.Close()

	return taskDirConfig, nil
}

func (fs *fileService) UnconpressPackage(zipFilePath string) (TaskDirConfig, error) {
	fs.logger.Infof("Unzipping solution package")

	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		fs.logger.Errorf("Failed to create temp directory. %s", err)
		return TaskDirConfig{}, err
	}

	err = os.Rename(zipFilePath, path+"/file.tar.gz")
	if err != nil {
		fs.logger.Errorf("Failed to rename file. %s", err)
		errRemove := utils.RemoveIO(path, true, true)
		if errRemove != nil {
			fs.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		}
		return TaskDirConfig{}, err
	}

	err = utils.ExtractTarGz(path+"/file.tar.gz", path)
	if err != nil {
		fs.logger.Errorf("Failed to extract file. %s", err)
		errRemove := utils.RemoveIO(path, true, true)
		if errRemove != nil {
			fs.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		}
		return TaskDirConfig{}, err
	}

	errRemove := utils.RemoveIO(path+"/file.tar.gz", false, false)
	if errRemove != nil {
		fs.logger.Errorf("Failed to remove file. %s", errRemove)
	}

	dirConfig := TaskDirConfig{
		UserSolutionDirPath: path,
		TaskFilesDirPath:    path + "/Task",
	}

	return dirConfig, nil
}

func (fs *fileService) StoreSolutionResult(solutionResult solution.SolutionResult, TaskFilesDirPath string, userId, taskId, submissionNumber int64) error {
	requestURL := fmt.Sprintf("%s/storeOutputs", fs.fileStorageUrl)
	outputsFolderPath := TaskFilesDirPath + "/" + solutionResult.OutputDir

	if solutionResult.StatusCode == solution.CompilationError {
		compilationErrorPath := TaskFilesDirPath + "/" + constants.CompileErrorFileName
		err := os.Rename(compilationErrorPath, outputsFolderPath+"/"+constants.CompileErrorFileName)
		if err != nil {
			return err
		}
	}

	err := utils.RemoveEmptyErrFiles(outputsFolderPath)
	if err != nil {
		return err
	}

	archiveFilePath, err := utils.TarGzFolder(outputsFolderPath)
	if err != nil {
		return err
	}

	file, err := os.Open(archiveFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("userID", strconv.Itoa(int(userId))); err != nil {
		return err
	}
	if err := writer.WriteField("taskID", strconv.Itoa(int(taskId))); err != nil {
		return err
	}
	if err := writer.WriteField("submissionNumber", strconv.Itoa(int(submissionNumber))); err != nil {
		return err
	}

	part, err := writer.CreateFormFile("archive", filepath.Base(archiveFilePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", requestURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := new(bytes.Buffer)
		_, err := bodyBytes.ReadFrom(resp.Body)
		if err != nil {
			fs.logger.Errorf("Failed to read response body: %s", err)
		}
		fs.logger.Errorf("Failed to store the solution result: %s", bodyBytes.String())
		return cutomErrors.ErrFailedToStoreSolution
	}

	return nil
}

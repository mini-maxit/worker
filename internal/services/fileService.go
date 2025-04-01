package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/mini-maxit/worker/internal/constants"
	cutomErrors "github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/internal/solution"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type FileService interface {
	HandleTaskPackage(taskID, userID, submissionNumber int64) (TaskDirConfig, error)
	UnconpressPackage(zipFilePath string) (TaskDirConfig, error)
	StoreSolutionResult(
		solutionResult solution.Result,
		TaskFilesDirPath string,
		userID, taskID, submissionNumber int64,
	) error
}

type fileService struct {
	fileStorageURL string
	logger         *zap.SugaredLogger
}

type TaskDirConfig struct {
	TaskFilesDirPath    string
	UserSolutionDirPath string
}

func NewFilesService(fileServiceURL string) FileService {
	logger := logger.NewNamedLogger("fileService")
	return &fileService{
		fileStorageURL: fileServiceURL,
		logger:         logger,
	}
}

func (fs *fileService) HandleTaskPackage(taskID, userID, submissionNumber int64) (TaskDirConfig, error) {
	fs.logger.Info("Handling task package")
	requestURL := fmt.Sprintf("%s/getSolutionPackage?taskID=%d&userID=%d&submissionNumber=%d",
		fs.fileStorageURL,
		taskID, userID, submissionNumber,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return TaskDirConfig{}, err
	}

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return TaskDirConfig{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
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
		// errRemove := utils.RemoveIO(path, true, true)
		// if errRemove != nil {
		// 	fs.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		// }
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

func (fs *fileService) StoreSolutionResult(
	solutionResult solution.Result,
	taskFilesDirPath string,
	userID, taskID, submissionNumber int64,
) error {
	outputsFolderPath := filepath.Join(taskFilesDirPath, solutionResult.OutputDir)

	if solutionResult.StatusCode == solution.CompilationError {
		if err := fs.moveCompilationError(taskFilesDirPath, outputsFolderPath); err != nil {
			return err
		}
	}

	if err := utils.RemoveEmptyErrFiles(outputsFolderPath); err != nil {
		return err
	}

	archiveFilePath, err := utils.TarGzFolder(outputsFolderPath)
	if err != nil {
		return err
	}

	return fs.sendArchiveToStorage(archiveFilePath, userID, taskID, submissionNumber)
}

func (fs *fileService) moveCompilationError(taskDir, outputDir string) error {
	src := filepath.Join(taskDir, constants.CompileErrorFileName)
	dst := filepath.Join(outputDir, constants.CompileErrorFileName)
	return os.Rename(src, dst)
}

func (fs *fileService) sendArchiveToStorage(
	archiveFilePath string,
	userID, taskID, submissionNumber int64,
) error {
	body, contentType, err := fs.prepareMultipartBody(archiveFilePath, userID, taskID, submissionNumber)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := fs.createRequestWithContext(ctx, body, contentType)
	if err != nil {
		return err
	}

	return fs.sendRequestAndHandleResponse(req)
}

func (fs *fileService) prepareMultipartBody(
	archiveFilePath string,
	userID, taskID, submissionNumber int64,
) (*bytes.Buffer, string, error) {
	file, err := os.Open(archiveFilePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	form := map[string]string{
		"userID":           strconv.FormatInt(userID, 10),
		"taskID":           strconv.FormatInt(taskID, 10),
		"submissionNumber": strconv.FormatInt(submissionNumber, 10),
	}

	for key, val := range form {
		if err := writer.WriteField(key, val); err != nil {
			return nil, "", err
		}
	}

	part, err := writer.CreateFormFile("archive", filepath.Base(archiveFilePath))
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", err
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return body, writer.FormDataContentType(), nil
}

func (fs *fileService) createRequestWithContext(
	ctx context.Context,
	body *bytes.Buffer,
	contentType string,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fs.fileStorageURL+"/storeOutputs", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return req, nil
}

func (fs *fileService) sendRequestAndHandleResponse(req *http.Request) error {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			fs.logger.Errorf("Failed to read response body: %s", err)
		}
		fs.logger.Errorf("Failed to store solution result: %s", buf.String())
		return cutomErrors.ErrFailedToStoreSolution
	}

	return nil
}

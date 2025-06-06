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
	"syscall"
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
	HandleTaskPackage(taskID, userID, submissionNumber int64) (*TaskDirConfig, error)
	UnconpressPackage(zipFilePath string) (*TaskDirConfig, error)
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

func (fs *fileService) HandleTaskPackage(taskID, userID, submissionNumber int64) (*TaskDirConfig, error) {
	fs.logger.Info("Handling task package")
	requestURL := fmt.Sprintf("%s/getSolutionPackage?taskID=%d&userID=%d&submissionNumber=%d",
		fs.fileStorageURL,
		taskID, userID, submissionNumber,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		fs.logger.Errorf("Failed to get solution package. %s", response.Status)
		bodyBytes, _ := io.ReadAll(response.Body)
		return nil, errors.New(string(bodyBytes))
	}

	id := uuid.New()
	filePath := fmt.Sprintf("/app/%s.tar.gz", id.String())
	file, err := os.Create(filePath)
	if err != nil {
		fs.logger.Errorf("Failed to create file. %s", err)
		return nil, err
	}

	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		fs.logger.Errorf("Failed to copy file. %s", err)
		return nil, err
	}

	taskDirConfig, err := fs.UnconpressPackage(filePath)
	if err != nil {
		return nil, err
	}

	err = file.Close()
	if err != nil {
		fs.logger.Errorf("Failed to close file. %s", err)
		return nil, err
	}

	return taskDirConfig, nil
}

func (fs *fileService) UnconpressPackage(zipFilePath string) (*TaskDirConfig, error) {
	fs.logger.Infof("Unzipping solution package")

	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		fs.logger.Errorf("Failed to create temp directory. %s", err)
		return nil, err
	}

	err = moveFile(zipFilePath, path+"/file.tar.gz")
	if err != nil {
		fs.logger.Errorf("Failed to rename file. %s", err)
		errRemove := utils.RemoveIO(path, true, true)
		if errRemove != nil {
			fs.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		}
		return nil, err
	}

	err = utils.ExtractTarGz(path+"/file.tar.gz", path)
	if err != nil {
		fs.logger.Errorf("Failed to extract file. %s", err)
		errRemove := utils.RemoveIO(path, true, true)
		if errRemove != nil {
			fs.logger.Errorf("Failed to remove temp directory. %s", errRemove)
		}
		return nil, err
	}

	errRemove := utils.RemoveIO(path+"/file.tar.gz", false, false)
	if errRemove != nil {
		fs.logger.Errorf("Failed to remove file. %s", errRemove)
	}

	dirConfig := TaskDirConfig{
		UserSolutionDirPath: path,
		TaskFilesDirPath:    path + "/Task",
	}

	return &dirConfig, nil
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

	if err := utils.RemoveExecutionResultFile(outputsFolderPath); err != nil {
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
	body, writer, err := fs.prepareMultipartBody(archiveFilePath, userID, taskID, submissionNumber)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := fs.createRequestWithContext(ctx, body, writer.FormDataContentType())
	if err != nil {
		return err
	}

	return fs.sendRequestAndHandleResponse(req)
}

func (fs *fileService) prepareMultipartBody(
	archiveFilePath string,
	userID, taskID, submissionNumber int64,
) (*bytes.Buffer, *multipart.Writer, error) {
	file, err := os.Open(archiveFilePath)
	if err != nil {
		return nil, nil, err
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
			return nil, nil, err
		}
	}

	part, err := writer.CreateFormFile("archive", filepath.Base(archiveFilePath))
	if err != nil {
		return nil, nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, nil, err
	}

	return body, writer, nil
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

// moveFile tries os.Rename and falls back to copy+delete on EXDEV.
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) {
		return copyAndRemove(src, dst)
	}
	return err
}

// copyAndRemove copies src to dst and removes src. Used as a fallback for moveFile on EXDEV.
func copyAndRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// ensure data is flushed
	if err := out.Sync(); err != nil {
		return err
	}
	// delete the original
	return os.Remove(src)
}


package storage

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

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/constants"
	cutomErrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type FileService interface {
	DownloadFile(fileLocation messages.FileLocation, destPath string) (string, error)
	UploadFile(filePath, bucket, objectKey string) error
}

type fileService struct {
	fileStorageURL string
	logger         *zap.SugaredLogger
}

func NewFilesService(fileServiceURL string) FileService {
	logger := logger.NewNamedLogger("fileService")
	return &fileService{
		fileStorageURL: fileServiceURL,
		logger:         logger,
	}
}

func (fs *fileService) DownloadFile(fileLocation messages.FileLocation, destPath string) (string, error) {

	fs.logger.Infof("Downloading file from bucket %s path %s", fileLocation.Bucket, fileLocation.Path)

	// Build request URL: {baseUrl}/buckets/:bucketName/:objectKey?metadataOnly=false
	requestURL := fmt.Sprintf("%s/buckets/%s/%s?metadataOnly=false", fs.fileStorageURL, fileLocation.Bucket, fileLocation.Path)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fs.logger.Errorf("Failed to download file. %s", resp.Status)
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", errors.New(string(bodyBytes))
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		fs.logger.Errorf("Failed to create destination directory: %s", err)
		return "", err
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		fs.logger.Errorf("Failed to create destination file: %s", err)
		return "", err
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		fs.logger.Errorf("Failed to copy downloaded file to destination: %s", err)
		return "", err
	}

	// Try to set file permissions to readable
	if err := outFile.Sync(); err != nil {
		fs.logger.Warnf("Failed to sync file to disk: %s", err)
	}
	if err := os.Chmod(destPath, 0o644); err != nil {
		// non-fatal
		fs.logger.Warnf("Failed to chmod file: %s", err)
	}

	fs.logger.Infof("File downloaded to %s", destPath)
	return destPath, nil
}

// TODO: implement
func (fs *fileService) UploadFile(filePath, bucket, objectKey string) error {
	fs.logger.Error("UploadFile method not implemented yet")
	return nil
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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/messages"
	"go.uber.org/zap"
)

type Storage interface {
	DownloadFile(fileLocation messages.FileLocation, destPath string) (string, error)
	UploadFile(filePath, bucket, objectKey string) error
}

type storage struct {
	fileStorageURL string
	logger         *zap.SugaredLogger
}

func NewStorage(fileServiceURL string) Storage {
	logger := logger.NewNamedLogger("storage")
	return &storage{
		fileStorageURL: fileServiceURL,
		logger:         logger,
	}
}

func (fs *storage) DownloadFile(fileLocation messages.FileLocation, destPath string) (string, error) {

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

func (fs *storage) UploadFile(filePath, bucket, objectKey string) error {
	fileName := filepath.Base(filePath)
	fs.logger.Infof("Uploading file %s to bucket %s path %s", fileName, bucket, objectKey)

	// Build request URL: {baseUrl}/buckets/:bucketName/:objectKey
	key := objectKey + "/" + fileName
	requestUrl := fmt.Sprintf("%s/buckets/%s/%s", fs.fileStorageURL, bucket, key)

	// Open the file to upload
	file, err := os.Open(filePath)
	if err != nil {
		fs.logger.Errorf("Failed to open file %s: %s", filePath, err)
		return err
	}
	defer file.Close()

	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodPost, requestUrl, file)
	if err != nil {
		fs.logger.Errorf("Failed to create upload request: %s", err)
		return err
	}

	// Set the content type
	req.Header.Set("Content-Type", "multipart/form-data")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fs.logger.Errorf("Failed to upload file: %s", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fs.logger.Errorf("Failed to upload file. %s", resp.Status)
		bodyBytes, _ := io.ReadAll(resp.Body)
		return errors.New(string(bodyBytes))
	}

	fs.logger.Infof("File uploaded successfully to %s", requestUrl)
	return nil
}

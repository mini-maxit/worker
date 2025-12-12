package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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
	// Build request URL: {baseUrl}/buckets/:bucketName/:objectKey?metadataOnly=false
	requestURL := fmt.Sprintf("%s/buckets/%s/%s?metadataOnly=false",
		fs.fileStorageURL,
		fileLocation.Bucket,
		fileLocation.Path)

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

	return destPath, nil
}

func (fs *storage) UploadFile(filePath, bucket, objectKey string) error {
	fileName := filepath.Base(filePath)
	// Build request URL: {baseUrl}/buckets/{bucket}/upload-multiple?prefix=<prefix>
	u, err := url.Parse(fmt.Sprintf("%s/buckets/%s/upload-multiple", fs.fileStorageURL, bucket))
	if err != nil {
		fs.logger.Errorf("Failed to parse upload URL: %s", err)
		return err
	}
	if objectKey != "" {
		q := u.Query()
		q.Set("prefix", objectKey)
		u.RawQuery = q.Encode()
	}
	requestUrl := u.String()

	// Open the file to upload
	file, err := os.Open(filePath)
	if err != nil {
		fs.logger.Errorf("Failed to open file %s: %s", filePath, err)
		return err
	}
	defer file.Close()

	// Prepare multipart form body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", fileName)
	if err != nil {
		fs.logger.Errorf("Failed to create multipart form file: %s", err)
		return err
	}

	if _, err := io.Copy(part, file); err != nil {
		fs.logger.Errorf("Failed to copy file into multipart body: %s", err)
		return err
	}

	if err := writer.Close(); err != nil {
		fs.logger.Errorf("Failed to close multipart writer: %s", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestUrl, body)
	if err != nil {
		fs.logger.Errorf("Failed to create upload request: %s", err)
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

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

	return nil
}

package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mini-maxit/file-storage/pkg/filestorage"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/messages"
	"go.uber.org/zap"
)

type Storage interface {
	DownloadFile(fileLocation messages.FileLocation, destPath string) error
	UploadFile(filePath, bucket, objectKey string) error
}

type storage struct {
	sdk    filestorage.FileStorage
	logger *zap.SugaredLogger
}

func NewStorage(fileServiceURL string) Storage {
	log := logger.NewNamedLogger("storage")
	sdk, err := filestorage.NewFileStorage(filestorage.FileStorageConfig{URL: fileServiceURL})
	if err != nil {
		log.Fatalf("Failed to create file storage SDK: %s", err)
	}
	return &storage{
		sdk:    sdk,
		logger: log,
	}
}

func (s *storage) DownloadFile(fileLocation messages.FileLocation, destPath string) error {
	data, err := s.sdk.GetFile(fileLocation.Bucket, fileLocation.Path)
	if err != nil {
		s.logger.Errorf("Failed to download file from bucket=%s path=%s: %s", fileLocation.Bucket, fileLocation.Path, err)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		s.logger.Errorf("Failed to create destination directory: %s", err)
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		s.logger.Errorf("Failed to create destination file: %s", err)
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		s.logger.Errorf("Failed to write file data: %s", err)
		return err
	}

	if err := f.Sync(); err != nil {
		s.logger.Warnf("Failed to sync file to disk: %s", err)
	}

	if err := os.Chmod(destPath, 0o644); err != nil {
		s.logger.Warnf("Failed to chmod file: %s", err)
	}

	return nil
}

func (s *storage) UploadFile(filePath, bucket, objectKey string) error {
	file, err := os.Open(filePath)
	if err != nil {
		s.logger.Errorf("Failed to open file %s: %s", filePath, err)
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	if err := s.sdk.UploadFile(bucket, objectKey, file); err != nil {
		s.logger.Errorf("Failed to upload file %s to bucket=%s objectKey=%s: %s", filePath, bucket, objectKey, err)
		return err
	}

	return nil
}

package utils

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mini-maxit/worker/internal/constants"
	e "github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/logger"
)

// Attemts to close the file, and panics if something goes wrong.
func CloseFile(file *os.File) {
	err := file.Close()
	if err != nil {
		// Check if the error is a PathError
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			log.Panicf("error during closing file %s. %s", pathErr.Path, pathErr.Error())
		}
		log.Panicf("unexpected error during closing file: %s", err.Error())
	}
}

// CopyFile copies a file from src to dst. It returns an error if any occurs during the copy.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return err
	}

	err = destinationFile.Sync()
	if err != nil {
		return err
	}

	return nil
}

// Checks if given elements is contained in given array.
func Contains[V string](array []V, value V) bool {
	for _, el := range array {
		if el == value {
			return true
		}
	}

	return false
}

// attempts to remove dir and optionaly its content. Can ignore error, for example if folder does not exist.
func RemoveIO(dir string, recursive, ignoreError bool) error {
	var err error
	if recursive {
		err = os.RemoveAll(dir)
	} else {
		err = os.Remove(dir)
	}

	if ignoreError {
		return nil
	}
	return err
}

// ExtractTarGz extracts a tar.gz archive to a given directory.
func ExtractTarGz(filePath string, baseFilePath string) error {
	logger := logger.NewNamedLogger("utils")
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	uncompressedStream, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() {
		if err := uncompressedStream.Close(); err != nil {
			logger.Errorf("error during closing uncompressed stream: %s", err.Error())
		}
	}()

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Clean and validate the path
		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return fmt.Errorf("invalid file path in archive: %s", cleanName)
		}

		targetPath := filepath.Join(baseFilePath, cleanName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(cleanName, 0755); err != nil {
				return err
			}

		case tar.TypeReg:
			if header.Size > constants.MaxFileSize {
				return fmt.Errorf("file too large: %s (%d bytes)", cleanName, header.Size)
			}

			err := handleRegularFileDecompression(tarReader, targetPath, header.Size)
			if err != nil {
				return err
			}

		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("symlinks not allowed: %s", cleanName)
		default:
			return fmt.Errorf("unsupported file type: %c", header.Typeflag)
		}
	}
	return nil
}

func handleRegularFileDecompression(tarReader *tar.Reader, targetPath string, size int64) error {
	if err := os.MkdirAll(path.Dir(targetPath), 0755); err != nil {
		return err
	}

	outFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	limitedReader := io.LimitReader(tarReader, size)
	if _, err := io.Copy(outFile, limitedReader); err != nil {
		return err
	}

	return nil
}

func TarGzFolder(srcDir string) (string, error) {
	logger := logger.NewNamedLogger("utils")
	absSrcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return "", err
	}
	parentDir := filepath.Dir(absSrcDir)
	outputFileName := filepath.Base(absSrcDir) + ".tar.gz"
	outputFilePath := filepath.Join(parentDir, outputFileName)

	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer func() {
		if err := gzWriter.Close(); err != nil {
			logger.Errorf("error during closing gzip writer: %s", err.Error())
		}
	}()

	tarWriter := tar.NewWriter(gzWriter)
	defer func() {
		err := tarWriter.Close()
		if err != nil {
			logger.Errorf("error during closing tar writer: %s", err.Error())
		}
	}()

	err = walkAndWriteToTar(absSrcDir, parentDir, tarWriter)
	if err != nil {
		return "", err
	}

	if err := tarWriter.Close(); err != nil {
		return "", err
	}
	if err := gzWriter.Close(); err != nil {
		return "", err
	}

	return outputFilePath, nil
}

func walkAndWriteToTar(absSrcDir, parentDir string, tarWriter *tar.Writer) error {
	return filepath.Walk(absSrcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := createTarHeader(fi, file, parentDir)
		if err != nil {
			return err
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		return writeFileToTar(file, tarWriter)
	})
}

func createTarHeader(fi os.FileInfo, file, parentDir string) (*tar.Header, error) {
	header, err := tar.FileInfoHeader(fi, file)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(parentDir, file)
	if err != nil {
		return nil, err
	}
	header.Name = relPath

	return header, nil
}

func writeFileToTar(file string, tarWriter *tar.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(tarWriter, f); err != nil {
		return err
	}

	return nil
}

func GetSolutionFileNameWithExtension(solutionFileBaseName string, languageType string) (string, error) {
	switch languageType {
	case "PYTHON":
		return solutionFileBaseName + ".py", nil
	case "CPP":
		return solutionFileBaseName + ".cpp", nil
	default:
		return "", e.ErrInvalidLanguageType
	}
}

// RemoveEmptyErrFiles removes empty .err files from the given directory.
func RemoveEmptyErrFiles(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := path.Join(dir, file.Name())
		if filepath.Ext(filePath) != ".err" {
			continue
		}
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return err
		}

		if fileInfo.Size() == 0 {
			err := os.Remove(filePath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

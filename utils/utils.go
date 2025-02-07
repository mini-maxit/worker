package utils

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
)

var errUnknownFileType = errors.New("unknown file type")
var errUnknownLanguageType = errors.New("unknown language type")

// Attemts to close the file, and panics if something goes wrong
func CloseFile(file *os.File) {
	err := file.Close()
	if err != nil {
		err := err.(*os.PathError)
		log.Panicf("error during closing file %s. %s", err.Path, err.Error())
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

// Checks if given elements is contained in given array
func Contains[V string](array []V, value V) bool {
	for _, el := range array {
		if el == value {
			return true
		}
	}

	return false
}

// attempts to remove dir and optionaly its content. Can ignore error, for example if folder does not exist
func RemoveIO(dir string, recursive, ignore_error bool) error {
	var err error
	if recursive {
		err = os.RemoveAll(dir)
	} else {
		err = os.Remove(dir)
	}

	if ignore_error {
		return nil
	}
	return err
}

// ExtractTarGz extracts a tar.gz archive to a given directory.
func ExtractTarGz(filePath string, baseFilePath string) error {

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	uncompressedStream, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer uncompressedStream.Close()

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			dirPath := path.Join(baseFilePath, header.Name)
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				return err
			}

		case tar.TypeReg:
			filePath := path.Join(baseFilePath, header.Name)
			if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(filePath)
			if err != nil {
				return err
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}

		default:
			return errUnknownFileType
		}
	}
	return nil
}

func TarGzFolder(srcDir string) (string, error) {
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
    defer gzWriter.Close()

    tarWriter := tar.NewWriter(gzWriter)
    defer tarWriter.Close()

    err = filepath.Walk(absSrcDir, func(file string, fi os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        header, err := tar.FileInfoHeader(fi, file)
        if err != nil {
            return err
        }

        relPath, err := filepath.Rel(parentDir, file)
        if err != nil {
            return err
        }
        header.Name = relPath

        if err := tarWriter.WriteHeader(header); err != nil {
            return err
        }

        if fi.IsDir() {
            return nil
        }

        f, err := os.Open(file)
        if err != nil {
            return err
        }
        defer f.Close()

        if _, err := io.Copy(tarWriter, f); err != nil {
            return err
        }

		return nil
	})
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

func GetSolutionFileNameWithExtension(SolutionFileBaseName string, languageType string) (string, error) {
	switch languageType {
	case "PYTHON":
		return SolutionFileBaseName + ".py", nil
	case "CPP":
		return SolutionFileBaseName + ".cpp", nil
	default:
		return "", errUnknownLanguageType
	}
}

// RemoveEmptyErrFiles removes empty .err files from the given directory
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

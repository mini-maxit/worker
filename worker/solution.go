package worker

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/google/uuid"

	"github.com/mini-maxit/worker/utils"
)

var errUnknownFileType = errors.New("unknown file type")


type TaskForRunner struct {
	BaseDir            string
	TempDir            string
	LanguageType       string
	LanguageVersion    string
	SolutionFileName   string
	InputDirName        string
	OutputDirName 		string
	TimeLimits 	       []float64
	MemoryLimits 	   []float64
}

type DirConfig struct {
	TempDir            string
    BaseDir            string `gorm:"column:dir_path"`
}


// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(task_id, user_id, submission_number int) (TaskForRunner, error) {
	var task TaskForRunner

	// Get the tar.gz file from the storage
	request_url := fmt.Sprintf("http://host.docker.internal:8080/getSolutionPackage?taskID=%d&userID=%d&submissionNumber=%d", task_id, user_id, submission_number)
	response, err := http.Get(request_url)
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

    // Get the directories configuration - temp dir and base dir
	dirConfig, err := handlePackage(filePath)
	if err != nil {
		return TaskForRunner{}, err
	}

	task.TempDir = dirConfig.TempDir
	task.BaseDir = dirConfig.BaseDir

	file.Close()
	utils.RemoveIO(filePath, false, false)

	return task, nil
}


func handlePackage(zipFilePath string) (DirConfig, error) {

	//Create a temp directory to store the unzipped files
	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		return DirConfig{}, err
	}

	// Move the zip file to the temp directory
	err = os.Rename(zipFilePath, path  + "/file.tar.gz")
	if err != nil {
		utils.RemoveIO(path, true, true)
		return DirConfig{}, err
	}

	// Unzip the file
	err = ExtractTarGz(path + "/file.tar.gz", path)
	if err != nil {
		utils.RemoveIO(path, true, true)
		return DirConfig{}, err
	}

	// Remove the zip file
	utils.RemoveIO(path + "/file.tar.gz", false, false)

	dirConfig := DirConfig{
		TempDir:            path,
		BaseDir:            path + "/Task",
	}

	return dirConfig, nil
}

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
			defer outFile.Close() // Close the file after copying

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}

		default:
			return errUnknownFileType
		}
	}
	return nil
}

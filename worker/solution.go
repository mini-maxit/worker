package worker

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"

	"github.com/mini-maxit/worker/utils"
	"gorm.io/gorm"
)

var errUnknownFileType = errors.New("unknown file type")


type TaskForRunner struct {
	BaseDir            string
	TempDir            string
	LanguageType       string
	LanguageVersion    string
	SolutionFileName   string
	StdinDir           string
	ExpectedOutputsDir string
	TimeLimits 	       []float64
	MemoryLimits 	   []float64
}

type InputOutputData struct {
	TimeLimit  		float64  `gorm:"column:time_limit"`
	MemoryLimit     float64  `gorm:"column:memory_limit"`
	Order 		    int      `gorm:"column:order"`
}

type SolutionConfig struct {
	LanguageType    string  `gorm:"column:language_type"`
	LanguageVersion string  `gorm:"column:language_version"`
	SolutionFileName string `gorm:"column:solution_file_name"`
}

type DirConfig struct {
	TempDir            string
    BaseDir            string `gorm:"column:dir_path"`
    StdinDir           string `gorm:"column:input_dir_path"`
    ExpectedOutputsDir string `gorm:"column:output_dir_path"`
}


// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *gorm.DB, task_id int, user_solution_id int) (TaskForRunner, error) {
	var task TaskForRunner

    // Get the directories configuration - base dir, input dir, output dir
	dirConfig, err := handlePackage()
	if err != nil {
		return TaskForRunner{}, err
	}

	task.TempDir = dirConfig.TempDir
	task.BaseDir = dirConfig.BaseDir
	task.StdinDir = dirConfig.StdinDir
	task.ExpectedOutputsDir = dirConfig.ExpectedOutputsDir

    // Get the soluton configuration - language type, language version, solution file name
    var solutionData SolutionConfig

	err = db.Table("user_solutions").Select("language_type, language_version, solution_file_name").Where("id = ?", user_solution_id).Scan(&solutionData).Error
	if err != nil {
		return TaskForRunner{}, err
	}

	task.LanguageType = solutionData.LanguageType
	task.LanguageVersion = solutionData.LanguageVersion
    task.SolutionFileName = solutionData.SolutionFileName

    // Get the time and memory limits for each input-output pair in order
	var ioData []InputOutputData

    err = db.Table("input_outputs").Select("time_limit, memory_limit, input_output_order").Where("task_id = ?", task_id).Order("input_output_order ASC").Find(&ioData).Error
    if err != nil {
        return TaskForRunner{}, err
    }

	for _, io := range ioData {
        task.TimeLimits = append(task.TimeLimits, io.TimeLimit)
        task.MemoryLimits = append(task.MemoryLimits, io.MemoryLimit)
    }

	return task, nil
}


func handlePackage() (DirConfig, error) {
	//this will be gathered from the file storage for now just place holder
	zipFile := "/app/file.tar.gz"

	//Create a temp directory to store the unzipped files
	path, err := os.MkdirTemp("", "temp")
	if err != nil {
		return DirConfig{}, err
	}

	// Move the zip file to the temp directory
	err = os.Rename(zipFile, path  + "/file.tgz")
	if err != nil {
		utils.RemoveDir(path, true, true)
		return DirConfig{}, err
	}


	// Unzip the file
	err = ExtractTarGz(path + "/file.tgz", path)
	if err != nil {
		utils.RemoveDir(path, true, true)
		return DirConfig{}, err
	}


	// Remove the zip file
	err = os.Remove(path + "/file.tgz")
	if err != nil {
		return DirConfig{}, err
	}


	dirConfig := DirConfig{
		TempDir:            path,
		BaseDir:            path + "/Task",
		StdinDir:           "inputs",
		ExpectedOutputsDir: "outputs",
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
			if err := os.Mkdir(baseFilePath + "/" + header.Name, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(baseFilePath + "/" + header.Name)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

		default:
			return errUnknownFileType
		}
	}
	return nil
}

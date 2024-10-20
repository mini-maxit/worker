package worker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)

// ErrInvalidFileCount is returned when the solution directory contains more than one solution file
var ErrInvalidFileCount = errors.New("expected exactly one file in the solution directory")

// ErrFileNotFound is returned when a file is not found
var ErrFileNotFound = errors.New("file not found")

// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *gorm.DB, task_id, user_id, user_solution_id, input_output_id int) (models.Task, error) {
	var task models.Task
	var dirPath string


	err := db.Table("tasks").Select("dir_path").Where("id = ?", task_id).Scan(&dirPath).Error
	if err != nil {
		return models.Task{}, err
	}


	err = db.Table("user_solutions").Select("compiler_type").Where("id = ?", user_solution_id).Scan(&task.CompilerType).Error
	if err != nil {
		return models.Task{}, err
	}

	var ioData models.InputOutputData

	err = db.Table("inputs_outputs").Select("time_limit", "memory_limit", "input_file_path", "output_file_path").Where("id = ?", input_output_id).Scan(&ioData).Error
	if err != nil {
		return models.Task{}, err
	}

	task.TimeLimit = ioData.TimeLimit
	task.MemoryLimit = ioData.MemoryLimit

	err = getFiles(dirPath, user_id, user_solution_id, ioData.InputFilePath, ioData.OutputFilePath, &task)
	if err != nil {
		return models.Task{}, err
	}

	return task, nil
}


// getFiles retrieves the solution, input, and output files from the file system
func getFiles(dirPath string, userID, userSolutionID int, inputsPath, outputsPath string, task *models.Task) error {
    solutionPath := fmt.Sprintf("%s/submissions/user%d/submition%d", dirPath, userID, userSolutionID)

    // Retrieve and read the solution file
    solutionFile, err := getSingleFile(solutionPath)
    if err != nil {
        return err
    }
    task.FileToExecute = models.File{Content: solutionFile}

    // Read input files
    if err := readFilesFromDir(inputsPath, &task.StdinFiles); err != nil {
        return err
    }

    // Read output files
    return readFilesFromDir(outputsPath, &task.ExpectedOutputs)
}

// getSingleFile retrieves a single non-directory file from the specified path
func getSingleFile(path string) ([]byte, error) {
    entries, err := os.ReadDir(path)
    if err != nil {
        return nil, err
    }

    var files []os.DirEntry
    for _, entry := range entries {
        if !entry.IsDir() && entry.Name() != ".DS_Store" {
            files = append(files, entry)
        }
    }

    if len(files) == 0 {
        return nil, ErrFileNotFound
    } else if len(files) > 1 {
        return nil, ErrInvalidFileCount
    }

    return os.ReadFile(fmt.Sprintf("%s/%s", path, files[0].Name()))
}

// readFilesFromDir reads all .txt files from the specified directory into the provided slice
func readFilesFromDir(dir string, fileList *[]models.File) error {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return err
    }

    for _, entry := range entries {
        if !entry.IsDir() && filepath.Ext(entry.Name()) == ".txt" {
            content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
            if err != nil {
                return err
            }
            *fileList = append(*fileList, models.File{Content: content})
        }
    }
    
    return nil
}


// Run the solution and return the result
func runSolution(task models.Task, user_solution_id int) (models.SolutionResult, error) {
	//TODO: Implement this function
	//For now, return a dummy result
	exampleSolutionResult := models.SolutionResult{
		UserSolutionId: user_solution_id,
		StatusCode:     0,
		Message:        "Success",
	}

	return exampleSolutionResult, nil
}
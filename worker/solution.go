package worker

import (
	"fmt"
	"os"
	"errors"
	"gorm.io/gorm"
	"github.com/mini-maxit/worker/models"
)


// ErrInvalidFileCount is returned when the solution directory contains more than one solution file
var ErrInvalidFileCount = errors.New("expected exactly one file in the solution directory")

// ErrFileNotFound is returned when a file is not found
var ErrFileNotFound = errors.New("file not found")

// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *gorm.DB, task_id, user_id, user_solution_id, input_output_id int) (models.Task, error) {
	var task models.Task
	var dirPath string

	// Fetch data with GORM
	err := db.Raw(`
		SELECT dir_path
		FROM tasks
		WHERE id = ?
	`, task_id).Scan(&dirPath).Error
	if err != nil {
		return models.Task{}, err
	}

	err = db.Raw(`
		SELECT compiler_type
		FROM user_solutions
		WHERE id = ?
	`, user_solution_id).Scan(&task.CompilerType).Error
	if err != nil {
		return models.Task{}, err
	}

	var ioData models.InputOutputData

	err = db.Raw(`
		SELECT time_limit, memory_limit, input_file_path, output_file_path
		FROM inputs_outputs
		WHERE id = ?
	`, input_output_id).Scan(&ioData).Error
	if err != nil {
		return models.Task{}, err
	}

	task.TimeLimit = ioData.TimeLimit
	task.MemoryLimit = ioData.MemoryLimit
	task.Stdin = ioData.InputFilePath
	task.ExpectedStdout = ioData.OutputFilePath
	if err != nil {
		return models.Task{}, err
	}

	err = getFiles(dirPath, user_id, user_solution_id, task.Stdin, task.ExpectedStdout, &task)
	if err != nil {
		return models.Task{}, err
	}

	return task, nil
}

// GetFiles retrieves the solution, input, and output files from the file system
func getFiles(dir_path string, user_id, user_solution_id int, input_path, output_path string, task *models.Task) error {
	path := fmt.Sprintf("%s/submissions/user%d/submition%d", dir_path, user_id, user_solution_id)

	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	var files []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != ".DS_Store" {
			files = append(files, entry)
		}
	}

	if len(files) == 0 {
		return ErrFileNotFound
	} else if len(files) > 1 {
		return ErrInvalidFileCount
	}

	solutionPath := fmt.Sprintf("%s/submissions/user%d/submition%d/%s", dir_path, user_id, user_solution_id, files[0].Name())

	// Read solution file
	solutionContent, err := os.ReadFile(solutionPath)
	if err != nil {
		return err
	}
	task.FileToExecute = string(solutionContent)

	// Read input file
	inputContent, err := os.ReadFile(input_path)
	if err != nil {
		return err
	}
	task.Stdin = string(inputContent)

	// Read output file
	outputContent, err := os.ReadFile(output_path)
	if err != nil {
		return err
	}
	task.ExpectedStdout = string(outputContent)

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
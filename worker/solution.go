package worker

import (
	"errors"
	"fmt"
	"os"

	"gorm.io/gorm"
)

// Solution struct (GORM Model)
type Solution struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	TaskId          int    `json:"task_id"`
	UserId          int    `json:"user_id"`
	UserSolutionId  int    `json:"user_solution_id"`
	InputOutputId   int    `json:"input_output_id"`
	Status          string `json:"status"`
}

type Task struct {
	FileToExecute  string
	CompilerType   string
	TimeLimit      int
	MemoryLimit    int
	Stdin          string
	ExpectedStdout string
}

type SolutionResult struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	UserSolutionId int    `json:"user_solution_id"`
	StatusCode     int    `json:"status_code"`
	Message        string `json:"message"`
}

// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *gorm.DB, task_id, user_id, user_solution_id, input_output_id int) (Task, error) {
	var task Task
	var dirPath string

	// Fetch data with GORM
	err := db.Raw(`
		SELECT dir_path
		FROM tasks
		WHERE id = ?
	`, task_id).Scan(&dirPath).Error
	if err != nil {
		return Task{}, err
	}

	err = db.Raw(`
		SELECT compiler_type
		FROM user_solutions
		WHERE id = ?
	`, user_solution_id).Scan(&task.CompilerType).Error
	if err != nil {
		return Task{}, err
	}

	var ioData struct {
		TimeLimit      int
		MemoryLimit    int
		InputFilePath  string
		OutputFilePath string
	}

	err = db.Raw(`
		SELECT time_limit, memory_limit, input_file_path, output_file_path
		FROM inputs_outputs
		WHERE id = ?
	`, input_output_id).Scan(&ioData).Error
	if err != nil {
		return Task{}, err
	}

	task.TimeLimit = ioData.TimeLimit
	task.MemoryLimit = ioData.MemoryLimit
	task.Stdin = ioData.InputFilePath
	task.ExpectedStdout = ioData.OutputFilePath
	if err != nil {
		return Task{}, err
	}

	err = getFiles(dirPath, user_id, user_solution_id, task.Stdin, task.ExpectedStdout, &task)
	if err != nil {
		return Task{}, err
	}

	return task, nil
}

// GetFiles retrieves the solution, input, and output files from the file system
func getFiles(dir_path string, user_id, user_solution_id int, input_path, output_path string, task *Task) error {
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

	if len(files) != 1 {
		return errors.New("expected exactly one file in the solution directory")
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
func runSolution(task Task, user_solution_id int) (SolutionResult, error) {
	//TODO: Implement this function
	//For now, return a dummy result
	exampleSolutionResult := SolutionResult{
		UserSolutionId: user_solution_id,
		StatusCode:     0,
		Message:        "Success",
	}

	return exampleSolutionResult, nil
}
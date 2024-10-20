package models

// Solution struct (GORM Model)
type Solution struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	TaskId          int    `json:"task_id"`
	UserId          int    `json:"user_id"`
	UserSolutionId  int    `json:"user_solution_id"`
	InputOutputId   int    `json:"input_output_id"`
	Status          string `json:"status"`
}

type File struct {
	Content []byte
}

type Task struct {
	FileToExecute  File
	CompilerType   string
	TimeLimit      int
	MemoryLimit    int
	StdinFiles     []File
	ExpectedOutputs []File
}

type SolutionResult struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	UserSolutionId int    `json:"user_solution_id"`
	StatusCode     int    `json:"status_code"`
	Message        string `json:"message"`
}

type InputOutputData struct {
		TimeLimit      int
		MemoryLimit    int
		InputFilePath  string
		OutputFilePath string
}
package models

// Solution struct is a model of a message that retrived from the queue
type Solution struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	TaskId          int    `json:"task_id"`
	UserId          int    `json:"user_id"`
	UserSolutionId  int    `json:"user_solution_id"`
	InputOutputId   int    `json:"input_output_id"`
	Status          string `json:"status"`
}

type Task struct {
	BaseDir			 	string
	SolutionFileName    string
	LanguageType        string
	LanguageVersion     string
	StdinDir            string
	ExpectedOutputsDir  string
}

type SolutionResult struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	UserSolutionId int    `json:"user_solution_id"`
	StatusCode     int    `json:"status_code"`
	Message        string `json:"message"`
}

type InputOutputData struct {
	StdinDir  		   string
	ExpectedOutputsDir string
}

type SolutionConfig struct {
	LanguageType    string
	LanguageVersion string
	SolutionFileName string
}

package worker

import (
	"github.com/mini-maxit/worker/models"
	"gorm.io/gorm"
)


// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *gorm.DB, task_id, user_solution_id, input_output_id int) (models.Task, error) {
	var task models.Task

    // Get the base directory path for the task
	err := db.Table("tasks").Select("dir_path").Where("id = ?", task_id).Scan(&task.BaseDir).Error
	if err != nil {
		return models.Task{}, err
	}

    // Get the soluton configuration - language type, language version, solution file name
    var solutionData models.SolutionConfig

	err = db.Table("user_solutions").Select("language_type, language_version, solution_file_name").Where("id = ?", user_solution_id).Scan(&solutionData).Error
	if err != nil {
		return models.Task{}, err
	}

	task.LanguageType = solutionData.LanguageType
	task.LanguageVersion = solutionData.LanguageVersion
    task.SolutionFileName = solutionData.SolutionFileName

    // Get the input dir path, and output dir path
	var ioData models.InputOutputData

	err = db.Table("inputs_outputs").Select("input_dir", "output_dir").Where("id = ?", input_output_id).Scan(&ioData).Error
	if err != nil {
		return models.Task{}, err
	}

    task.StdinDir = ioData.StdinDir
    task.ExpectedOutputsDir = ioData.ExpectedOutputsDir

	return task, nil
}

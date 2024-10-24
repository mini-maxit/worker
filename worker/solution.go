package worker
import (
	"gorm.io/gorm"
)


type TaskForRunner struct {
	BaseDir            string
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
    BaseDir            string `gorm:"column:dir_path"`
    StdinDir           string `gorm:"column:input_dir_path"`
    ExpectedOutputsDir string `gorm:"column:output_dir_path"`
}



// GetDataForSolutionRunner retrieves the data needed to run the solution
func getDataForSolutionRunner(db *gorm.DB, task_id int, user_solution_id int, input_output_ids []int) (TaskForRunner, error) {
	var task TaskForRunner

	var dirConfig DirConfig

    // Get the directories configuration - base dir, input dir, output dir
	err := db.Table("tasks").Select("dir_path, input_dir_path, output_dir_path").Where("id = ?", task_id).Scan(&dirConfig).Error
	if err != nil {
		return TaskForRunner{}, err
	}

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

    err = db.Table("inputs_outputs").Select("time_limit, memory_limit, order").Where("id IN ?", input_output_ids).Order("order ASC").Find(&ioData).Error
    if err != nil {
        return TaskForRunner{}, err
    }

	for _, io := range ioData {
        task.TimeLimits = append(task.TimeLimits, io.TimeLimit)
        task.MemoryLimits = append(task.MemoryLimits, io.MemoryLimit)
    }

	return task, nil
}

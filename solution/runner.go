package solution

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/verifier"
)

var ErrInvalidLanguageType = errors.New("invalid language type")

type LanguageType int

var languageTypeMap = map[string]LanguageType{
    "CPP": CPP,
}

var languageExtensionMap = map[LanguageType]string{
	CPP: ".cpp",
}

func GetSolutionFileNameWithExtension(solutionName string, language LanguageType) (string, error) {
	if extension, ok := languageExtensionMap[language]; ok {
		return fmt.Sprintf("%s%s", solutionName, extension), nil
	}
	return "", ErrInvalidLanguageType
}

func StringToLanguageType(s string) (LanguageType, error) {
    if lt, ok := languageTypeMap[s]; ok {
        return lt, nil
    }
    return 0, ErrInvalidLanguageType
}

const (
	CPP LanguageType = iota + 1
)

type SolutionRunner interface {
	RunSolution() SolutionResult
}

type Runner struct {
}

type LanguageConfig struct {
	Type    LanguageType // Which type of language (cpp, c, py)
	Version string       // Version should be valid version supported by the executor
}

func (r *Runner) RunSolution(solution *Solution, messageID string) SolutionResult {
	logger := logger.NewNamedLogger("runner")

	logger.Infof("Initializing executor [MsgID: %s]", messageID)
	// Init appropriate executor
	var exec executor.Executor
	var err error
	switch solution.Language.Type {
	case CPP:
		logger.Infof("Initializing C++ executor [MsgID: %s]", messageID)
		exec, err = executor.NewCppExecutor(solution.Language.Version, messageID)
	default:
		logger.Errorf("Invalid language type supplied [MsgID: %s]", messageID)
		return SolutionResult{
			Success: false,
			Message: "invalid language type supplied",
		}
	}
	if err != nil {
		logger.Errorf("Error occured during initialization [MsgID: %s]", messageID)
		return SolutionResult{
			Success:    false,
			StatusCode: InternalError,
			Code:       getStatus(InternalError),
			Message:    err.Error(),
		}
	}

	logger.Infof("Creating user output directory [MsgID: %s]", messageID)

	userOutputDir := "user-output"
	err = os.Mkdir(fmt.Sprintf("%s/%s", solution.BaseDir, userOutputDir), os.ModePerm)
	if err != nil {
		logger.Errorf("Error creating user output directory [MsgID: %s]: %s", messageID, err.Error())
		return SolutionResult{
			Success:    false,
			StatusCode: InternalError,
			Code:       getStatus(InternalError),
			Message:    err.Error(),
		}
	}

	logger.Infof("Creaed user output directory [MsgID: %s]", messageID)

	var filePath string
	if exec.IsCompiled() {
		solutionFilePath := fmt.Sprintf("%s/%s", solution.BaseDir, solution.SolutionFileName)
		logger.Infof("Compiling solution file %s [MsgID: %s]", solutionFilePath, messageID)
		filePath, err = exec.Compile(solutionFilePath, solution.BaseDir, messageID)
		if err != nil {
			logger.Errorf("Error compiling solution file %s [MsgID: %s]: %s", solutionFilePath, messageID, err.Error())
			return SolutionResult{
				OutputDir: userOutputDir,
				Success:    false,
				StatusCode: Failed,
				Code:       getStatus(Failed),
				Message:    err.Error(),
			}
		}
	} else {
		filePath = solution.SolutionFileName
	}

	inputPath := fmt.Sprintf("%s/%s", solution.BaseDir, solution.InputDir)
	logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		logger.Errorf("Error reading input files from %s [MsgID: %s]: %s", inputPath, messageID, err.Error())
		return SolutionResult{
			OutputDir: userOutputDir,
			Success:    false,
			StatusCode: Failed,
			Code:       getStatus(Failed),
			Message:    err.Error(),
		}
	}

	logger.Infof("Executing solution [MsgID: %s]", messageID)
	verifier := verifier.NewDefaultVerifier()
	testCases := make([]TestResult, len(inputFiles))
	solutionSuccess := true
	for i, inputPath := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", solution.BaseDir, userOutputDir, (i+1))
		stderrPath := fmt.Sprintf("%s/%s/%d.err", solution.BaseDir, userOutputDir, (i+1)) // May be dropped in the future
		_ = exec.ExecuteCommand(filePath, messageID, executor.CommandConfig{StdinPath: inputPath, StdoutPath: outputPath, StderrPath: stderrPath})

		// Compare output with expected output
		logger.Infof("Comparing output %s with expected output [MsgID: %s]",outputPath ,messageID)
		expectedFilePath := fmt.Sprintf("%s/%s/%d.out", solution.BaseDir, solution.OutputDir, (i+1))
		result, difference, err := verifier.CompareOutput(outputPath, expectedFilePath)
		if err != nil {
			logger.Errorf("Error comparing output %s with expected output [MsgID: %s]: %s", outputPath, messageID, err.Error())
			solutionSuccess = false
			difference = err.Error()
		}
		if !result {
			solutionSuccess = false
		}
		testCases[i] = TestResult{
			Passed:       result,
			ErrorMessage: difference,
			Order: (i+1),
		}
	}

	logger.Infof("Solution executed successfully [MsgID: %s]", messageID)
	return SolutionResult{
		OutputDir:   userOutputDir,
		Success:     solutionSuccess,
		StatusCode:  Success,
		Code:        getStatus(Success),
		Message:     "solution executed successfully",
		TestResults: testCases,
	}
}

func (r *Runner) parseInputFiles(inputDir string) ([]string, error) {
	dirEntries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".in") {
			result = append(result, fmt.Sprintf("%s/%s", inputDir, entry.Name()))
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty input directory, verify task files")
	}

	return result, nil

}

func NewRunner() *Runner {
	return &Runner{}
}

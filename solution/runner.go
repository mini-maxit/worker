package solution

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/verifier"
)

type LanguageType int

var languageTypeMap = map[string]LanguageType{
    "CPP": CPP,
}

func StringToLanguageType(s string) (LanguageType, error) {
    if lt, ok := languageTypeMap[s]; ok {
        return lt, nil
    }
    return 0, fmt.Errorf("invalid language type: %s", s)
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

func (r *Runner) RunSolution(solution *Solution) SolutionResult {
	// Init appropriate executor
	var exec executor.Executor
	var err error
	switch solution.Language.Type {
	case CPP:
		exec, err = executor.NewCppExecutor(solution.Language.Version)
	default:
		return SolutionResult{
			Success: false,
			Message: "invalid language type supplied",
		}
	}
	if err != nil {
		return SolutionResult{
			Success:    false,
			StatusCode: InternalError,
			Code:       getStatus(InternalError),
			Message:    err.Error(),
		}
	}

	var filePath string
	if exec.IsCompiled() {
		solutionFilePath := fmt.Sprintf("%s/%s", solution.BaseDir, solution.SolutionFileName)
		log.Printf("compiling %s", solutionFilePath)
		filePath, err = exec.Compile(solutionFilePath, solution.BaseDir)
		if err != nil {
			log.Printf("error compiling %s. %s", solution.SolutionFileName, err.Error())
			return SolutionResult{
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
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		log.Fatalf("error reading input directory. %s", err.Error())
	}

	userOutputDir := "user-output"
	err = os.Mkdir(fmt.Sprintf("%s/%s", solution.BaseDir, userOutputDir), os.ModePerm)
	if err != nil {
		log.Fatalf("error creating user output directory. %s", err.Error())
	}

	verifier := verifier.NewDefaultVerifier()
	testCases := make([]TestResult, len(inputFiles))
	solutionSuccess := true
	for i, inputPath := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", solution.BaseDir, userOutputDir, i)
		stderrPath := fmt.Sprintf("%s/%s/%d.err", solution.BaseDir, userOutputDir, i) // May be dropped in the future
		_ = exec.ExecuteCommand(filePath, executor.CommandConfig{StdinPath: inputPath, StdoutPath: outputPath, StderrPath: stderrPath})

		// Compare output with expected output
		expectedFilePath := fmt.Sprintf("%s/%s/%d.out", solution.BaseDir, solution.OutputDir, i)
		result, difference, err := verifier.CompareOutput(outputPath, expectedFilePath)
		if err != nil {
			log.Printf("error comparing output. %s", err.Error())
			solutionSuccess = false
			difference = err.Error()
		}
		if !result {
			solutionSuccess = false
		}
		testCases[i] = TestResult{
			InputFile:    inputPath,
			ExpectedFile: expectedFilePath,
			ActualFile:   outputPath,
			Passed:       result,
			ErrorMessage: difference,
		}
	}

	for _, testCase := range testCases {
		if !testCase.Passed {
			solutionSuccess = false
			break
		}
	}

	return SolutionResult{
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

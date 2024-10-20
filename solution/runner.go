package solution

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/utils"
	"github.com/mini-maxit/worker/verifier"
)

type LanguageType int

const (
	PYTHON LanguageType = iota
	CPP
)

type SolutionRunner interface {
	RunSolution() SolutionResult
}

type SolutionResult struct {
	Success     bool         // wether solution was without any error
	StatusCode  int          // any status code in case of error or success
	Code        string       // any code in case of error or success
	Message     string       // any information message in case of error is error message
	TestResults []TestResult // test results in case of error or success
}
type TestResult struct {
	InputFile    string // Path to the input file used for the test
	ExpectedFile string // Path to the expected output file
	ActualFile   string // Path to the actual output file produced by the solution
	Passed       bool   // Whether the test passed or failed
	ErrorMessage string // Error message in case of failure (if any)
}

type Runner struct {
}

type LanguageConfig struct {
	Type    LanguageType // Which type of language (cpp, c, py)
	Version string       // Version should be valid version supported by the executor
}

// TODO: replace ALL Fatalf with approproiate SolutionResult, to inform end user about the error instead of crushing like oioioi))))
// Input path contains points to directory with input files. File name should be of format {number}.in
//
// language is the language of the solution
//
// baseDir is the base directory where solution file is stored together with compiled file if applicable
//
// solutionFileName is the name of the solution file
//
// inputDir is the directory containing input files inside baseDir
//
// outputDir is the directory where correct output files are stored
func (r *Runner) RunSolution(language LanguageConfig, baseDir string, solutionFileName string, inputDir string, outputDir string) SolutionResult {

	// Init appropriate executor
	var exec executor.Executor
	var err error
	switch language.Type {
	// case PYTHON:
	// 	exec, err = executor.NewPyExecutor(language.Version)
	case CPP:
		exec, err = executor.NewCppExecutor(language.Version)
	default:
		return SolutionResult{
			Success: false,
			Message: "invalid language type supplied",
		}
	}
	if err != nil {
		return SolutionResult{
			Success: false,
			Message: err.Error(),
		}
	}

	var filePath string
	if exec.IsCompiled() {
		solutionFilePath := fmt.Sprintf("%s/%s", baseDir, solutionFileName)
		log.Printf("compiling %s", solutionFilePath)
		filePath, err = exec.Compile(solutionFilePath, baseDir)
		if err != nil {
			log.Printf("error compiling %s. %s", solutionFileName, err.Error())
			return SolutionResult{
				Success: false,
				Message: err.Error(),
			}
		}
	} else {
		filePath = solutionFileName
	}

	inputPath := fmt.Sprintf("%s/%s", baseDir, inputDir)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		log.Fatalf("error reading input directory. %s", err.Error())
	}
	userOutputDir := "user-output"
	err = os.Mkdir(fmt.Sprintf("%s/%s", baseDir, userOutputDir), os.ModePerm)
	if err != nil {
		log.Fatalf("error creating user output directory. %s", err.Error())
	}
	verifier := verifier.NewDefaultVerifier()
	testCases := make([]TestResult, len(inputFiles))
	solutionSuccess := true
	for i, inputPath := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", baseDir, userOutputDir, i)
		stderrPath := fmt.Sprintf("%s/%s/%d.err", baseDir, userOutputDir, i)
		_ = exec.ExecuteCommand(filePath, executor.CommandConfig{StdinSource: inputPath, StdoutPath: outputPath, StderrPath: stderrPath})
		// if execResult.StatusCode != 0 {
		// 	log.Printf("error executing %s. %s", filePath, execResult.String())
		// 	continue
		// }

		// Compare output with expected output
		expectedFilePath := fmt.Sprintf("%s/%s/%d.out", baseDir, outputDir, i)
		result, difference, err := verifier.CompareOutput(outputPath, expectedFilePath)
		if err != nil {
			log.Printf("error comparing output. %s", err.Error())
			continue
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

	return SolutionResult{
		Success:     solutionSuccess,
		StatusCode:  0,
		Code:        "",
		Message:     "",
		TestResults: testCases,
	}
}

func (r *Runner) RemoveDir(dir string) {
	log.Printf("removing directory %s", dir)
	err := utils.RemoveDir(dir, true, false)
	if err != nil {
		log.Printf("error occured while removing directory %s. %s", dir, err.Error())
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

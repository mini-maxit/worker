package solution

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/HermanPlay/maxit-worker/executor"
	"github.com/HermanPlay/maxit-worker/utils"
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
	Success bool   // wether solution was without any error
	Message string // any information message in case of error is error message
}

type Runner struct {
}

type LanguageConfig struct {
	Type    LanguageType // Which type of language (cpp, c, py)
	Version string       // Version should be valid version supported by the executor
}

// TODO: replace ALL Fatalf with approproiate SolutionResult, to inform end user about the error instead of crushing like oioioi))))
// Input path contains points to directory with input files. File name should be of format {number}.in
func (r *Runner) RunSolution(language LanguageConfig, solutionFilePath string, inputPath string, outputPath string) SolutionResult {

	// Create directory for executor output
	dir, err := os.MkdirTemp(os.TempDir(), "foldername")
	if err != nil {
		log.Fatalf("could not create temp dir. %s", err.Error())
	}
	defer r.RemoveDir(dir)
	log.Printf("temp dir: %s", dir)

	// Init appropriate executor
	var exec executor.Executor
	switch language.Type {
	case PYTHON:
		exec, err = executor.NewPyExecutor(language.Version)
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

	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		log.Fatalf("error reading input directory. %s", err.Error())
	}
	inputFiles = append(inputFiles, "test")
	exec.ExecuteCommand("ls")

	return SolutionResult{}
}

func (r *Runner) RemoveDir(dir string) {
	err := utils.RemoveDir(dir, true, false)
	if err != nil {
		log.Printf("error occured while removing directory %s. %s", dir, err.Error())
	}

}

func (r *Runner) parseInputFiles(inputPath string) ([]string, error) {
	dirEntries, err := os.ReadDir(inputPath)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".in") {
			result = append(result, entry.Name())
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

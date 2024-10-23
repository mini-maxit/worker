package main

import (
	"fmt"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/solution"
)

func main() {
	runner := solution.NewRunner()
	lang_conf := solution.LanguageConfig{
		Type:    solution.CPP,
		Version: executor.CPP_17,
	}

	dir := "test-paczka"

	solution := &solution.Solution{
		Language:         lang_conf,
		BaseDir:          dir,
		SolutionFileName: "test-cpp.cpp",
		InputDir:         "input",
		OutputDir:        "output",
	}
	result := runner.RunSolution(solution)
	for _, test := range result.TestResults {
		fmt.Println(test)
	}
}

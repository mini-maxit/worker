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
	// Create directory for executor output
	// dir, err := os.MkdirTemp(os.TempDir(), "foldername")
	// if err != nil {
	// 	log.Fatalf("could not create temp dir. %s", err.Error())
	// }
	// defer r.RemoveDir(dir)
	// log.Printf("temp dir: %s", dir)
	dir := "test-paczka"

	solution := runner.RunSolution(lang_conf, dir, "test-cpp.cpp", "input", "output")
	for _, test := range solution.TestResults {
		fmt.Println(test)
	}
}

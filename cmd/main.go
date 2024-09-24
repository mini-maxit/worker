package main

import (
	"github.com/HermanPlay/maxit-worker/solution"
)

func main() {
	runner := solution.NewRunner()
	lang_conf := solution.LanguageConfig{
		Type:    solution.PYTHON,
		Version: "3.11",
	}
	runner.RunSolution(lang_conf)

}

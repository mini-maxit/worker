package solution

import (
	"testing"
)

func TestRunnerRunCommandInvalidLanguage(t *testing.T) {
	runner := NewRunner()

	lang_conf := LanguageConfig{
		Type:    -1,
		Version: "version",
	}
	solution := &Solution{
		Language:         lang_conf,
		BaseDir:          "",
		SolutionFileName: "",
		InputDir:         "",
		OutputDir:        "",
	}
	solutionResult := runner.RunSolution(solution)

	if solutionResult.Success {
		t.Fatalf("solution succeded with invalid language type")
	}

	if solutionResult.Message == "" {
		t.Fatalf("empty error message when invalid language")
	}

	lang_conf.Type = CPP

	solution = &Solution{
		Language:         lang_conf,
		BaseDir:          "",
		SolutionFileName: "",
		InputDir:         "",
		OutputDir:        "",
	}
	solutionResult = runner.RunSolution(solution)

	if solutionResult.Success {
		t.Fatalf("solution succeded with invalid language version")
	}

	if solutionResult.Message == "" {
		t.Fatalf("empty error message when invalid language")
	}

}

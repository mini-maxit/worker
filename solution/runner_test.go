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
	solutionResult := runner.RunSolution(lang_conf, "", "", "")

	if solutionResult.Success {
		t.Fatalf("solution succeded with invalid language type")
	}

	if solutionResult.Message == "" {
		t.Fatalf("empty error message when invalid language")
	}

	lang_conf.Type = PYTHON

	solutionResult = runner.RunSolution(lang_conf, "", "", "")

	if solutionResult.Success {
		t.Fatalf("solution succeded with invalid language version")
	}

	if solutionResult.Message == "" {
		t.Fatalf("empty error message when invalid language")
	}

}

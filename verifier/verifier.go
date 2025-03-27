package verifier

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mini-maxit/worker/internal/constants"
)

// Returns true if the output and expected output are the same
// Returns false otherwise
// Returns an error if an error occurs
type Verifier interface {
	CompareOutput(outputPath, expectedOutputPath, stderrPath string) (bool, error)
}

type DefaultVerifier struct {
	flags string
}

// CompareOutput compares the output file with the expected output file.
// It returns true if the files are the same, false otherwise.
// It returns a string of differences if there are any.
// It returns an error if any issue occurs during the comparison.
func (dv *DefaultVerifier) CompareOutput(outputPath, expectedFilePath, stderrPath string) (bool, error) {
	diffCmd := exec.Command("diff", dv.flags, outputPath, expectedFilePath)
	diffCmdOutput, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, fmt.Errorf("error opening output file %s: %w", outputPath, err)
	}
	defer diffCmdOutput.Close()

	diffCmd.Stdout = diffCmdOutput
	err = diffCmd.Run()
	diffExitCode := diffCmd.ProcessState.ExitCode()
	if err != nil {
		if diffExitCode == constants.ExitCodeDifference {
			return false, nil
		}
		return false, fmt.Errorf("error running diff command: %w", err)
	}

	return true, nil
}

func NewDefaultVerifier(flags string) Verifier {
	return &DefaultVerifier{
		flags: flags,
	}
}

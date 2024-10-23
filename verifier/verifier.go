package verifier

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Returns true if the output and expected output are the same
// Returns false otherwise
// Returns an error if an error occurs
type Verifier interface {
	CompareOutput(outputPath string, expectedOutputPath string) (bool, string, error)
}

type DefaultVerifier struct{}

// CompareOutput compares the output file with the expected output file.
// It returns true if the files are the same, false otherwise.
// It returns a string of differences if there are any.
// It returns an error if any issue occurs during the comparison.
func (dv *DefaultVerifier) CompareOutput(outputPath string, expectedOutputPath string) (bool, string, error) {
	outputFile, err := os.Open(outputPath)
	if err != nil {
		return false, "", fmt.Errorf("failed to open output file: %v", err)
	}
	defer outputFile.Close()

	expectedFile, err := os.Open(expectedOutputPath)
	if err != nil {
		return false, "", fmt.Errorf("failed to open expected output file: %v", err)
	}
	defer expectedFile.Close()

	outputScanner := bufio.NewScanner(outputFile)
	expectedScanner := bufio.NewScanner(expectedFile)

	var differences strings.Builder
	isIdentical := true
	lineNumber := 1

	for outputScanner.Scan() && expectedScanner.Scan() {
		outputLine := outputScanner.Text()
		expectedLine := expectedScanner.Text()

		if outputLine != expectedLine {
			isIdentical = false
			differences.WriteString(fmt.Sprintf("Difference at line %d:\n", lineNumber))
			differences.WriteString(fmt.Sprintf("Output:   %s\n", outputLine))
			differences.WriteString(fmt.Sprintf("Expected: %s\n\n", expectedLine))
		}
		lineNumber++
	}

	// Check if there are remaining lines in either file
	for outputScanner.Scan() {
		isIdentical = false
		outputLine := outputScanner.Text()
		differences.WriteString(fmt.Sprintf("Extra line in output at line %d:\n", lineNumber))
		differences.WriteString(fmt.Sprintf("Output: %s\n\n", outputLine))
		lineNumber++
	}

	for expectedScanner.Scan() {
		isIdentical = false
		expectedLine := expectedScanner.Text()
		differences.WriteString(fmt.Sprintf("Missing line in output at line %d:\n", lineNumber))
		differences.WriteString(fmt.Sprintf("Expected: %s\n\n", expectedLine))
		lineNumber++
	}

	if err := outputScanner.Err(); err != nil {
		return false, "", fmt.Errorf("error reading output file: %v", err)
	}

	if err := expectedScanner.Err(); err != nil {
		return false, "", fmt.Errorf("error reading expected output file: %v", err)
	}

	return isIdentical, differences.String(), nil
}

func NewDefaultVerifier() Verifier {
	return &DefaultVerifier{}
}

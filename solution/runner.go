package solution

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/verifier"
	"go.uber.org/zap"
)

var ErrInvalidLanguageType = errors.New("invalid language type")

type LanguageType int

var languageTypeMap = map[string]LanguageType{
	"CPP": CPP,
}

var languageExtensionMap = map[LanguageType]string{
	CPP: ".cpp",
}

func GetSolutionFileNameWithExtension(solutionName string, language LanguageType) (string, error) {
	if extension, ok := languageExtensionMap[language]; ok {
		return fmt.Sprintf("%s%s", solutionName, extension), nil
	}
	return "", ErrInvalidLanguageType
}

func StringToLanguageType(s string) (LanguageType, error) {
	if lt, ok := languageTypeMap[strings.ToUpper(s)]; ok {
		return lt, nil
	}
	return 0, ErrInvalidLanguageType
}

const (
	CPP LanguageType = iota + 1
)

type SolutionRunner interface {
	RunSolution() SolutionResult
}

type Runner struct {
	logger *zap.SugaredLogger
}

type LanguageConfig struct {
	Type    LanguageType // Which type of language (cpp, c, py)
	Version string       // Version should be valid version supported by the executor
}

func (r *Runner) RunSolution(solution *Solution, messageID string) SolutionResult {

	r.logger.Infof("Initializing executor [MsgID: %s]", messageID)
	// Init appropriate executor
	var exec executor.Executor
	var err error
	switch solution.Language.Type {
	case CPP:
		r.logger.Infof("Initializing C++ executor [MsgID: %s]", messageID)
		exec, err = executor.NewCppExecutor(solution.Language.Version, messageID)
	default:
		r.logger.Errorf("Invalid language type supplied [MsgID: %s]", messageID)
		return SolutionResult{
			Success: false,
			Message: "invalid language type supplied",
		}
	}
	if err != nil {
		r.logger.Errorf("Error occured during initialization [MsgID: %s]", messageID)
		return SolutionResult{
			Success:    false,
			StatusCode: InitializationError,
			Code:       InitializationError.String(),
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Creating user output directory [MsgID: %s]", messageID)

	userOutputDir := "user-output"
	err = os.Mkdir(fmt.Sprintf("%s/%s", solution.BaseDir, userOutputDir), os.ModePerm)
	if err != nil {
		r.logger.Errorf("Error creating user output directory [MsgID: %s]: %s", messageID, err.Error())
		return SolutionResult{
			Success:    false,
			StatusCode: InternalError,
			Code:       InternalError.String(),
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Creaed user output directory [MsgID: %s]", messageID)

	var filePath string
	if exec.IsCompiled() {
		solutionFilePath := fmt.Sprintf("%s/%s", solution.BaseDir, solution.SolutionFileName)
		r.logger.Infof("Compiling solution file %s [MsgID: %s]", solutionFilePath, messageID)
		filePath, err = exec.Compile(solutionFilePath, solution.BaseDir, messageID)
		if err != nil {
			r.logger.Errorf("Error compiling solution file %s [MsgID: %s]: %s", solutionFilePath, messageID, err.Error())
			return SolutionResult{
				OutputDir:  userOutputDir,
				Success:    false,
				StatusCode: CompilationError,
				Code:       CompilationError.String(),
				Message:    err.Error(),
			}
		}
	} else {
		filePath = solution.SolutionFileName
	}

	inputPath := fmt.Sprintf("%s/%s", solution.BaseDir, solution.InputDir)
	r.logger.Infof("Reading input files from %s [MsgID: %s]", inputPath, messageID)
	inputFiles, err := r.parseInputFiles(inputPath)
	if err != nil {
		r.logger.Errorf("Error reading input files from %s [MsgID: %s]: %s", inputPath, messageID, err.Error())
		return SolutionResult{
			OutputDir:  userOutputDir,
			Success:    false,
			StatusCode: Failed,
			Code:       Failed.String(),
			Message:    err.Error(),
		}
	}

	r.logger.Infof("Executing solution [MsgID: %s]", messageID)
	verifier := verifier.NewDefaultVerifier()
	testCases := make([]TestResult, len(inputFiles))
	solutionSuccess := true
	solutionStatus := Success
	solutionMessage := "solution executed successfully"
	for i, inputPath := range inputFiles {
		outputPath := fmt.Sprintf("%s/%s/%d.out", solution.BaseDir, userOutputDir, (i + 1))
		stderrPath := fmt.Sprintf("%s/%s/%d.err", solution.BaseDir, userOutputDir, (i + 1)) // May be dropped in the future
		commandConfig := executor.CommandConfig{
			StdinPath:   inputPath,
			StdoutPath:  outputPath,
			StderrPath:  stderrPath,
			TimeLimit:   solution.TimeLimits[i],
			MemoryLimit: solution.MemoryLimits[i],
		}

		execResult := exec.ExecuteCommand(filePath, messageID, commandConfig)
		switch execResult.ExitCode {
		case executor.Success:
			r.logger.Infof("Comparing output %s with expected output [MsgID: %s]", outputPath, messageID)
			expectedFilePath := fmt.Sprintf("%s/%s/%d.out", solution.BaseDir, solution.OutputDir, (i + 1))
			result, difference, err := verifier.CompareOutput(outputPath, expectedFilePath)
			if err != nil {
				r.logger.Errorf("Error comparing output %s with expected output [MsgID: %s]: %s", outputPath, messageID, err.Error())
				solutionSuccess = false
				difference = err.Error()
			}
			if !result {
				solutionSuccess = false
			}
			testCases[i] = TestResult{
				Passed:       result,
				ErrorMessage: difference,
				Order:        (i + 1),
			}
		case executor.TimeLimitExceeded:
			r.logger.Errorf("Time limit exceeded while executing solution [MsgID: %s]", messageID)
			testCases[i] = TestResult{
				Passed:       false,
				ErrorMessage: "time limit exceeded",
				Order:        (i + 1),
			}
			solutionSuccess = false
			solutionStatus = RuntimeError
			solutionMessage = "Some test cases failed due to time limit exceeded"
		case executor.MemoryLimitExceeded:
			r.logger.Errorf("Memory limit exceeded while executing solution [MsgID: %s]", messageID)
			testCases[i] = TestResult{
				Passed:       false,
				ErrorMessage: "memory limit exceeded",
				Order:        (i + 1),
			}
			solutionSuccess = false
			solutionStatus = RuntimeError
			solutionMessage = "Some test cases failed due to memory limit exceeded"
		default:
			r.logger.Errorf("Error executing solution [MsgID: %s]: %s", messageID, execResult.Message)
			testCases[i] = TestResult{
				Passed:       false,
				ErrorMessage: execResult.Message,
				Order:        (i + 1),
			}
			solutionSuccess = false
			solutionStatus = RuntimeError
			solutionMessage = "Some test cases failed due to runtime error"
		}
	}

	r.logger.Infof("Solution executed successfully [MsgID: %s]", messageID)
	return SolutionResult{
		OutputDir:   userOutputDir,
		Success:     solutionSuccess,
		StatusCode:  solutionStatus,
		Code:        solutionStatus.String(),
		Message:     solutionMessage,
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
	logger := logger.NewNamedLogger("runner")

	return &Runner{logger: logger}
}

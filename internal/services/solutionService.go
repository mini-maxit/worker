package services

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/mini-maxit/worker/compiler"
	"github.com/mini-maxit/worker/executor"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
	"go.uber.org/zap"
)

type SolutionService interface {
	RemoveExecutionResultFile(dir string) error
	PrepareSolutionFilePath(
		taskFilesDirPath string,
		solutionFileName string,
		solutionCompiler compiler.Compiler,
		messageID string,
	) (string, error)
	SetupOutputErrorFiles(taskDirPath, outputDirName string, numberOfFiles int) error
	ReadExecutionResultFile(filePath string, numberOfTest int) ([]*executor.ExecutionResult, error)
}

type solutionService struct {
	logger *zap.SugaredLogger
}

func NewSolutionService() SolutionService {
	logger := logger.NewNamedLogger("solutionService")
	return &solutionService{
		logger: logger,
	}
}

func (s *solutionService) RemoveExecutionResultFile(dir string) error {
	s.logger.Infof("Removing execution result file(s) in directory: %s", dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		s.logger.Errorf("Failed to read directory %s: %v", dir, err)
		return err
	}

	removed := false
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := path.Join(dir, file.Name())
		if filepath.Base(filePath) != constants.ExecResultFileName {
			continue
		}

		err := os.Remove(filePath)
		if err != nil {
			s.logger.Errorf("Failed to remove execution result file %s: %v", filePath, err)
			return err
		}
		s.logger.Infof("Removed execution result file: %s", filePath)
		removed = true
	}

	if !removed {
		s.logger.Warnf("No execution result file named %s found in directory: %s", constants.ExecResultFileName, dir)
	}

	return nil
}

func (s *solutionService) PrepareSolutionFilePath(
	taskFilesDirPath string,
	solutionFileName string,
	solutionCompiler compiler.Compiler,
	messageID string,
) (string, error) {
	s.logger.Infof("Preparing solution file path for %s (messageID: %s)", solutionFileName, messageID)
	var filePath string
	var err error
	solutionFilePath := fmt.Sprintf("%s/%s", taskFilesDirPath, solutionFileName)
	if solutionCompiler.RequiresCompilation() {
		s.logger.Infof("Compiling solution file: %s", solutionFilePath)
		filePath, err = solutionCompiler.Compile(solutionFilePath, taskFilesDirPath, messageID)
		if err != nil {
			s.logger.Errorf("Compilation failed for %s: %v", solutionFilePath, err)
			return "", err
		}
		s.logger.Infof("Compilation successful. Compiled file path: %s", filePath)
	} else {
		filePath = solutionFilePath
		s.logger.Infof("No compilation required. Using file path: %s", filePath)
	}

	return filePath, nil
}

func (s *solutionService) SetupOutputErrorFiles(
	taskDirPath,
	outputDirName string,
	numberOfFiles int) error {
	s.logger.Infof("Setting up output and error files in %s/%s for %d test cases",
		taskDirPath,
		outputDirName,
		numberOfFiles)
	for i := 1; i <= numberOfFiles; i++ {
		outputFilePath := fmt.Sprintf("%s/%s/%d.out", taskDirPath, outputDirName, i)
		outputFile, err := os.Create(outputFilePath)
		if err != nil {
			s.logger.Errorf("Failed to create output file %s: %v", outputFilePath, err)
			return fmt.Errorf("failed to create output file: %w", err)
		}
		outputFile.Close()
		s.logger.Debugf("Created output file: %s", outputFilePath)

		errorFilePath := fmt.Sprintf("%s/%s/%d.err", taskDirPath, outputDirName, i)
		errorFile, err := os.Create(errorFilePath)
		if err != nil {
			s.logger.Errorf("Failed to create error file %s: %v", errorFilePath, err)
			return fmt.Errorf("failed to create error file: %w", err)
		}
		errorFile.Close()
		s.logger.Debugf("Created error file: %s", errorFilePath)
	}
	s.logger.Infof("Successfully set up output and error files.")
	return nil
}

func (s *solutionService) ReadExecutionResultFile(
	filePath string,
	numberOfTest int,
) ([]*executor.ExecutionResult, error) {
	s.logger.Infof("Reading execution result file: %s for %d test(s)", filePath, numberOfTest)
	file, err := os.Open(filePath)
	if err != nil {
		s.logger.Errorf("Failed to open execution result file %s: %v", filePath, err)
		return nil, err
	}
	defer file.Close()

	results := make([]*executor.ExecutionResult, numberOfTest)
	for i := range results {
		var exitCode int
		var execTime float64
		_, err := fmt.Fscanf(file, "%d %f\n", &exitCode, &execTime)
		if err != nil {
			s.logger.Errorf("Failed to parse execution result at index %d: %v", i, err)
			return nil, err
		}
		results[i] = &executor.ExecutionResult{
			ExitCode: exitCode,
			ExecTime: execTime,
		}
		s.logger.Debugf("Parsed result %d: ExitCode=%d, ExecTime=%f", i+1, exitCode, execTime)
	}
	s.logger.Infof("Successfully read all execution results from %s", filePath)
	return results, nil
}

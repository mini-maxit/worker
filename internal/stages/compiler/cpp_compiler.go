package compiler

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/internal/logger"
	"go.uber.org/zap"
)

type CppCompiler struct {
	version string
	logger  *zap.SugaredLogger
}

func (e *CppCompiler) RequiresCompilation() bool {
	return true
}

// For now compile allows only one file.
func (e *CppCompiler) Compile(sourceFilePath, outFilePath, compErrFilePath, messageID string) error {
	e.logger.Infof("Compiling %s [MsgID: %s]", sourceFilePath, messageID)
	// Correctly pass the command and its arguments as separate strings.
	versionFlag := "-std=" + e.version
	cmd := exec.Command("g++", "-o", outFilePath, versionFlag, sourceFilePath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	e.logger.Infof("Running command [MsgID: %s]", messageID)
	cmdErr := cmd.Run()
	if cmdErr != nil {
		// Save stderr to a file
		e.logger.Errorf("Error during compilation. %s [MsgID: %s]", cmdErr.Error(), messageID)
		e.logger.Infof("Creating stderr file [MsgID: %s]", messageID)
		file, err := os.Create(compErrFilePath)
		if err != nil {
			e.logger.Errorf("Could not create stderr file. %s [MsgID: %s]", err.Error(), messageID)
			return err
		}

		e.logger.Infof("Writing error to stderr file [MsgID: %s]", messageID)
		_, err = file.Write(stderr.Bytes())
		if err != nil {
			e.logger.Errorf("Error writing to file. %s [MsgID: %s]", err.Error(), messageID)
			return err
		}
		e.logger.Infof("Closing stderr file [MsgID: %s]", messageID)
		err = file.Close()
		if err != nil {
			e.logger.Errorf("Error closing file. %s [MsgID: %s]", err.Error(), messageID)
			return err
		}
		e.logger.Infof("Compilation error saved to %s [MsgID: %s]", compErrFilePath, messageID)
		return cmdErr
	}
	e.logger.Infof("Compilation successful [MsgID: %s]", messageID)
	return nil
}

func NewCppCompiler(version, messageID string) (*CppCompiler, error) {
	logger := logger.NewNamedLogger("cpp-compiler")
	versionFlag, err := languages.GetVersionFlag(languages.CPP, version)
	if err != nil {
		logger.Errorf("Failed to get version flag. %s [MsgID: %s]", err.Error(), messageID)
		return nil, err
	}
	return &CppCompiler{
		version: versionFlag,
		logger:  logger}, nil
}

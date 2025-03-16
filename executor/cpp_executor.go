package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type CppExecutor struct {
	version string
	config  *ExecutorConfig
	logger  *zap.SugaredLogger
}

func (e *CppExecutor) ExecuteCommand(command, messageID string, commandConfig CommandConfig) *ExecutionResult {
	restrictedCmd, rootToChrootExecPath, err := copyExecutableToChrootAndRestric(command, commandConfig)
	if err != nil {
		e.logger.Errorf("Could not copy executable to chroot and restrict. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: constants.ExitCodeInternalError,
			Message:  fmt.Sprintf("could not copy executable to chroot and restrict. %s", err.Error()),
		}
	}

	defer func() {
		if err := utils.RemoveIO(rootToChrootExecPath, false, false); err != nil {
			e.logger.Errorf("Could not remove executable from chroot. %s [MsgID: %s]", err.Error(), messageID)
		}
	}()

	// Open io files
	ioConfig, err := setupIOFiles(commandConfig.StdinPath, commandConfig.StdoutPath, commandConfig.StderrPath)
	if err != nil {
		e.logger.Errorf("Could not open io files. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: constants.ExitCodeInternalError,
			Message:  fmt.Sprintf("could not open io files. %s", err.Error()),
		}
	}

	// Set the io files for the command
	restrictedCmd.Stdin = ioConfig.Stdin
	restrictedCmd.Stdout = ioConfig.Stdout
	restrictedCmd.Stderr = ioConfig.Stderr

	defer func() {
		utils.CloseFile(ioConfig.Stdin)
		utils.CloseFile(ioConfig.Stdout)
		utils.CloseFile(ioConfig.Stderr)
	}()

	// Execute the command
	e.logger.Infof("Executing command [MsgID: %s]", messageID)
	runErr := restrictedCmd.Run()
	exitCode := restrictedCmd.ProcessState.ExitCode()

	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
		e.logger.Infof("Appending error message to stderr file [MsgID: %s] %s", messageID, errorMessage)
		if _, err := ioConfig.Stderr.WriteString(errorMessage); err != nil {
			e.logger.Errorf("Could not write error message to stderr file. %s [MsgID: %s]", err.Error(), messageID)
		}
	}

	return &ExecutionResult{
		ExitCode: exitCode,
		Message:  errorMessage,
	}
}

func (e *CppExecutor) String() string {
	var out bytes.Buffer

	out.WriteString("CppExecutor{")
	out.WriteString("Version: " + e.version + ", ")
	out.WriteString("Config: " + e.config.String())
	out.WriteString("}")

	return out.String()

}

func (e *CppExecutor) RequiresCompilation() bool {
	return true
}

// For now compile allows only one file
func (e *CppExecutor) Compile(sourceFilePath, dir, messageID string) (string, error) {
	e.logger.Infof("Compiling %s [MsgID: %s]", sourceFilePath, messageID)
	outFilePath := fmt.Sprintf("%s/solution", dir)
	// Correctly pass the command and its arguments as separate strings
	cmd := exec.Command("g++", "-o", outFilePath, fmt.Sprintf("-std=%s", e.version), sourceFilePath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	e.logger.Infof("Running command [MsgID: %s]", messageID)
	cmdErr := cmd.Run()
	if cmdErr != nil {
		// Save stderr to a file
		e.logger.Errorf("Error during compilation. %s [MsgID: %s]", cmdErr.Error(), messageID)
		e.logger.Infof("Creating stderr file [MsgID: %s]", messageID)
		errPath := fmt.Sprintf("%s/%s", dir, constants.CompileErrorFileName)
		file, err := os.Create(errPath)
		if err != nil {
			e.logger.Errorf("Could not create stderr file. %s [MsgID: %s]", err.Error(), messageID)
			return "", err
		}

		e.logger.Infof("Writing error to sdterr file [MsgID: %s]", messageID)
		_, err = file.Write(stderr.Bytes())
		if err != nil {
			e.logger.Errorf("Error writing to file. %s [MsgID: %s]", err.Error(), messageID)
			return "", err
		}
		e.logger.Infof("Closing stderr file [MsgID: %s]", messageID)
		err = file.Close()
		if err != nil {
			e.logger.Errorf("Error closing file. %s [MsgID: %s]", err.Error(), messageID)
			return "", err
		}
		e.logger.Infof("Compilation error saved to %s [MsgID: %s]", errPath, messageID)
		return errPath, cmdErr
	}
	e.logger.Infof("Compilation successful [MsgID: %s]", messageID)
	return outFilePath, nil
}

func NewCppExecutor(version, messageID string) (*CppExecutor, error) {
	logger := logger.NewNamedLogger("cpp-executor")
	versionFlag, err := languages.GetVersionFlag(languages.CPP, version)
	if err != nil {
		logger.Errorf("Failed to get version flag. %s [MsgID: %s]", err.Error(), messageID)
		return nil, err
	}
	return &CppExecutor{
		version: versionFlag,
		logger:  logger}, nil
}

package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/uuid"
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
	id := uuid.New()
	chrootExecName := id.String()
	rootToChrootExecPath := fmt.Sprintf("%s/%s", commandConfig.ChrootDirPath, chrootExecName)
	// Copy the executable to the chroot
	err := utils.CopyFile(command, rootToChrootExecPath)
	if err != nil {
		e.logger.Errorf("Could not copy executable to chroot. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: -1,
			Message:  fmt.Sprintf("could not copy executable to chroot. %s", err.Error()),
		}
	}
	defer func() {
		if err := utils.RemoveIO(rootToChrootExecPath, false, false); err != nil {
			e.logger.Errorf("Could not remove executable from chroot. %s [MsgID: %s]", err.Error(), messageID)
		}
	}()

	// Chane the permissions of the executable
	err = os.Chmod(rootToChrootExecPath, 0755)
	if err != nil {
		e.logger.Errorf("Could not change permissions of the executable. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: -1,
			Message:  fmt.Sprintf("could not change permissions of the executable. %s", err.Error()),
		}
	}

	// Restrict command to run in chroot with time limit
	restrictedCmd := restrictCommand(chrootExecName, commandConfig)

	// Open stdout file
	e.logger.Infof("Opening stdout file [MsgID: %s]", messageID)
	stdout, err := os.OpenFile(commandConfig.StdoutPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		e.logger.Errorf("Could not open stdout file. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: constants.ExitCodeInternalError,
			Message:  fmt.Sprintf("could not open stdout file. %s", err.Error()),
		}
	}
	restrictedCmd.Stdout = stdout
	defer utils.CloseFile(stdout)

	// Open stderr file
	e.logger.Infof("Opening stderr file [MsgID: %s]", messageID)
	stderr, err := os.OpenFile(commandConfig.StderrPath, os.O_RDWR|os.O_CREATE|os.O_SYNC, 0755)
	if err != nil {
		e.logger.Errorf("Could not open stderr file. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: constants.ExitCodeInternalError,
			Message:  fmt.Sprintf("could not open stderr file. %s", err.Error()),
		}
	}
	restrictedCmd.Stderr = stderr
	defer utils.CloseFile(stderr)

	// Handle stdin if supplied
	if len(commandConfig.StdinPath) > 0 {
		e.logger.Infof("Opening stdin file [MsgID: %s]", messageID)

		// Open stdin
		stdin, err := os.OpenFile(commandConfig.StdinPath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			e.logger.Errorf("Could not open stdin file. %s [MsgID: %s]", err.Error(), messageID)
			return &ExecutionResult{
				ExitCode: constants.ExitCodeInternalError,
				Message:  fmt.Sprintf("could not open stdin file in chroot. %s", err.Error()),
			}
		}
		restrictedCmd.Stdin = stdin
		defer utils.CloseFile(stdin)
	}

	// Execute the command
	e.logger.Infof("Executing command [MsgID: %s]", messageID)
	runErr := restrictedCmd.Run()
	exitCode := restrictedCmd.ProcessState.ExitCode()

	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
		e.logger.Infof("Appending error message to stderr file [MsgID: %s] %s", messageID, errorMessage)
		if _, err := stderr.WriteString(errorMessage); err != nil {
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

func (e *CppExecutor) IsCompiled() bool {
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

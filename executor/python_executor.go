package executor

import (
	"bytes"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type PythonExecutor struct {
	version string
	config  *Config
	logger  *zap.SugaredLogger
}

func (e *PythonExecutor) ExecuteCommand(command, messageID string, commandConfig CommandConfig) *ExecutionResult {
	e.logger.Infof("COmmmand: %s", command)
	restrictedCmd, rootToChrootExecPath, err := CopyExecutableToChrootAndRestric(command, commandConfig)
	if err != nil {
		e.logger.Errorf("Could not copy executable to chroot and restrict. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: constants.ExitCodeInternalError,
			Message:  err.Error(),
		}
	}

	e.logger.Infof("Root to chroot exec path: %s", rootToChrootExecPath)

	defer func() {
		if err := utils.RemoveIO(rootToChrootExecPath, false, false); err != nil {
			e.logger.Errorf("Could not remove executable from chroot. %s [MsgID: %s]", err.Error(), messageID)
		}
	}()

	// Open io files
	ioConfig, err := SetupIOFiles(commandConfig.StdinPath, commandConfig.StdoutPath, commandConfig.StderrPath)
	if err != nil {
		e.logger.Errorf("Could not open io files. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			ExitCode: constants.ExitCodeInternalError,
			Message:  err.Error(),
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

	// Append the version flag to the command
	args := restrictedCmd.Args
	args = append(args[:len(args)-1], e.version, args[len(args)-1])
	restrictedCmd.Args = args

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

func (e *PythonExecutor) RequiresCompilation() bool {
	return false
}

func (e *PythonExecutor) Compile(_, _, _ string) (string, error) {
	return "", errors.ErrDoesNotRequireCompilation
}

func (e *PythonExecutor) String() string {
	var out bytes.Buffer

	out.WriteString("PythonExecutor{")
	out.WriteString("Version: " + e.version + ", ")
	out.WriteString("Config: " + e.config.String())
	out.WriteString("}")

	return out.String()
}

func NewPythonExecutor(version, messageID string) (*PythonExecutor, error) {
	logger := logger.NewNamedLogger("PythonExecutor")
	versionFlag, err := languages.GetVersionFlag(languages.Python, version)
	if err != nil {
		logger.Errorf("Failed to get version flag. %s [MsgID: %s]", err.Error(), messageID)
		return nil, err
	}

	return &PythonExecutor{
		version: versionFlag,
		logger:  logger}, nil
}

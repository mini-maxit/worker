package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/utils"
)

const (
	CPP_11 = "11"
	CPP_14 = "14"
	CPP_17 = "17"
	CPP_20 = "20"
)

var CPP_AVAILABLE_VERSION = []string{CPP_11, CPP_14, CPP_17, CPP_20}

var ErrInvalidVersion = fmt.Errorf("invalid version supplied")

type CppExecutor struct {
	version string
	config  *ExecutorConfig
}

func (e *CppExecutor) ExecuteCommand(command, messageID string, commandConfig CommandConfig) *ExecutionResult {
	logger := logger.NewNamedLogger("cpp-executor")
	// Prepare command for execution
	cmd := exec.Command(command)

	logger.Infof("Opening stdout file [MsgID: %s]", messageID)
	stdout, err := os.OpenFile(commandConfig.StdoutPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		logger.Errorf("Could not open stdout file. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not open stdout file. %s", err.Error()),
		}
	}
	cmd.Stdout = stdout
	defer utils.CloseFile(stdout)

	logger.Infof("Opening stderr file [MsgID: %s]", messageID)
	stderr, err := os.OpenFile(commandConfig.StderrPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		logger.Errorf("Could not open stderr file. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not open stderr file. %s", err.Error()),
		}
	}
	cmd.Stderr = stderr
	defer utils.CloseFile(stderr)

	// Provide stdin if supplied
	if len(commandConfig.StdinPath) > 0 {
		logger.Infof("Opening stdin file [MsgID: %s]", messageID)
		stdin, err := os.Open(commandConfig.StdinPath)
		if err != nil {
			logger.Errorf("Could not open stdin file. %s [MsgID: %s]", err.Error(), messageID)
			return &ExecutionResult{
				StatusCode: ErInternalError,
				Message:    fmt.Sprintf("could not open stdin file. %s", err.Error()),
			}
		}
		cmd.Stdin = stdin
		defer utils.CloseFile(stdin)
	}

	// Execute command
	logger.Infof("Executing command [MsgID: %s]", messageID)
	err = cmd.Run()
	if err != nil {
		logger.Errorf("Could not run the command. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not run the command. %s", err.Error()),
		}
	}

	var statusCode ExecutorStatusCode
	switch cmd.ProcessState.ExitCode() {
	case -1:
		statusCode = ErSignalRecieved
	case 0:
		statusCode = ErSuccess
	default:
		logger.Errorf("Command exited with unknown status code. %d [MsgID: %s]", cmd.ProcessState.ExitCode(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("command exited with unknown status code. %d", cmd.ProcessState.ExitCode()),
		}
	}

	logger.Infof("Command executed successfully [MsgID: %s]", messageID)
	return &ExecutionResult{
		StatusCode: statusCode,
		Message:    "Command executed successfully",
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
	logger := logger.NewNamedLogger("cpp-executor")

	logger.Infof("Compiling %s [MsgID: %s]", sourceFilePath, messageID)
	// Prepare command for execution
	var versionFlag string
	switch e.version {
	case CPP_11:
		versionFlag = "c++11"
	case CPP_14:
		versionFlag = "c++14"
	case CPP_17:
		versionFlag = "c++17"
	case CPP_20:
		versionFlag = "c++20"
	default:
		logger.Errorf("Invalid C++ version supplied [MsgID: %s]", messageID)
		return "", ErrInvalidVersion
	}
	outFilePath := fmt.Sprintf("%s/solution", dir)
	// Correctly pass the command and its arguments as separate strings
	cmd := exec.Command("g++", "-o", outFilePath, fmt.Sprintf("-std=%s", versionFlag), sourceFilePath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	logger.Infof("Running command [MsgID: %s]", messageID)
	cmdErr := cmd.Run()
	if cmdErr != nil {
		// Save stderr to a file
		logger.Errorf("Error during compilation. %s [MsgID: %s]", cmdErr.Error(), messageID)
		logger.Infof("Creating stderr file [MsgID: %s]", messageID)
		errPath := fmt.Sprintf("%s/%s", dir, CompileErrorFileName)
		file, err := os.Create(errPath)
		if err != nil {
			logger.Errorf("Could not create stderr file. %s [MsgID: %s]", err.Error(), messageID)
			return "", err
		}
    
		logger.Infof("Writing error to sdterr file [MsgID: %s]", messageID)
		_, err = file.Write(stderr.Bytes())
		if err != nil {
			logger.Errorf("Error writing to file. %s [MsgID: %s]", err.Error(), messageID)
			return "", err
		}
		logger.Infof("Closing stderr file [MsgID: %s]", messageID)
		err = file.Close()
		if err != nil {
			logger.Errorf("Error closing file. %s [MsgID: %s]", err.Error(), messageID)
			return "", err
		}
		logger.Infof("Compilation error saved to %s [MsgID: %s]", errPath, messageID)
		return errPath, cmdErr
	}
	logger.Infof("Compilation successful [MsgID: %s]", messageID)
	return outFilePath, nil
}

func NewCppExecutor(version, messageID string) (*CppExecutor, error) {
	logger := logger.NewNamedLogger("cpp-executor")

	logger.Infof("Initializing C++ executor [MsgID: %s]", messageID)
	if !utils.Contains(CPP_AVAILABLE_VERSION, version) {
		logger.Errorf("Invalid version supplied [MsgID: %s]", messageID)
		return nil, fmt.Errorf("invalid version supplied. got=%s, availabe=%s", version, CPP_AVAILABLE_VERSION)
	}
	return &CppExecutor{version: version}, nil
}

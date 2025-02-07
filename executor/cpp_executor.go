package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	"github.com/mini-maxit/worker/logger"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
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
	logger  *zap.SugaredLogger
}

func (e *CppExecutor) ExecuteCommand(command, messageID string, commandConfig CommandConfig) *ExecutionResult {
	e.logger.Infof("Executing command [MsgID: %s]", messageID)

	id := uuid.New()
	chrootExecPath := id.String()
	rootToChrootExecPath := fmt.Sprintf("%s/%s", BaseChrootDir, chrootExecPath)

	// Copy the executable to the chroot
	err := utils.CopyFile(command, rootToChrootExecPath)
	if err != nil {
		e.logger.Errorf("Could not copy executable to chroot. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not copy executable to chroot. %s", err.Error()),
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
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not change permissions of the executable. %s", err.Error()),
		}
	}

	// Restrict command to run in chroot with time limit
	restrictedCmd := restrictCommand(chrootExecPath, commandConfig.TimeLimit)

	// Open stdout file
	e.logger.Infof("Opening stdout file [MsgID: %s]", messageID)
	stdout, err := os.OpenFile(commandConfig.StdoutPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		e.logger.Errorf("Could not open stdout file. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not open stdout file. %s", err.Error()),
		}
	}
	restrictedCmd.Stdout = stdout
	defer utils.CloseFile(stdout)

	// Open stderr file
	e.logger.Infof("Opening stderr file [MsgID: %s]", messageID)
	stderr, err := os.OpenFile(commandConfig.StderrPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		e.logger.Errorf("Could not open stderr file. %s [MsgID: %s]", err.Error(), messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not open stderr file. %s", err.Error()),
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
				StatusCode: ErInternalError,
				Message:    fmt.Sprintf("could not open stdin file in chroot. %s", err.Error()),
			}
		}
		restrictedCmd.Stdin = stdin
		defer utils.CloseFile(stdin)
	}

	// Execute the command
	e.logger.Infof("Executing command [MsgID: %s]", messageID)
	runErr := restrictedCmd.Run()

	if runErr == nil {
		var statusCode ExecutorStatusCode
		switch restrictedCmd.ProcessState.ExitCode() {
		case -1:
			statusCode = ErSignalRecieved
		case 0:
			statusCode = ErSuccess
		default:
			e.logger.Errorf("Command exited with unknown status code. %d [MsgID: %s]", restrictedCmd.ProcessState.ExitCode(), messageID)
			return &ExecutionResult{
				StatusCode: ErInternalError,
				Message:    fmt.Sprintf("command exited with unknown status code. %d", restrictedCmd.ProcessState.ExitCode()),
			}
		}

		e.logger.Infof("Finished executing command with status code: %d [MsgID: %s]", statusCode, messageID)
		return &ExecutionResult{
			StatusCode: statusCode,
			Message:    "Command executed successfully",
		}
	} else {
		//Check if the command timed out or was blocked by chroot
		exitError := runErr.(*exec.ExitError)

		e.logger.Infof("Command exited with status code %d [MsgID: %s]", exitError.ExitCode(), messageID)
		if exitError.ExitCode() == ExitCodeTimeout {
			e.logger.Errorf("Command timed out [MsgID: %s]", messageID)
			timeOutContent := fmt.Sprintf("The command timed out after %d seconds\n", commandConfig.TimeLimit)
			fmt.Fprint(stderr, timeOutContent)
			return &ExecutionResult{
				StatusCode: ErTimeout,
				Message:    "The command timed out",
			}
		}

		// Read stderr file
		stderrContent, err := os.ReadFile(stderr.Name())
		if err != nil {
			e.logger.Errorf("Could not read stderr file. %s [MsgID: %s]", err.Error(), messageID)
			return &ExecutionResult{
				StatusCode: ErInternalError,
				Message:    fmt.Sprintf("could not read stderr file. %s", err.Error()),
			}
		}
		errorOutput := string(stderrContent)
		e.logger.Errorf("Error executing command. %s [MsgID: %s]", errorOutput, messageID)

		// Check if the command was blocked by chroot
		if strings.Contains(errorOutput, "chroot") {
			e.logger.Errorf("Command blocked due to chroot restrictions [MsgID: %s]", messageID)
			return &ExecutionResult{
				StatusCode: ErJailed,
				Message:    fmt.Sprintf("command blocked due to chroot restrictions. %s", errorOutput),
			}
		}

		e.logger.Errorf("Error executing command. %s [MsgID: %s]", errorOutput, messageID)
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("error executing command. %s", errorOutput),
		}
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
		e.logger.Errorf("Invalid C++ version supplied [MsgID: %s]", messageID)
		return "", ErrInvalidVersion
	}
	outFilePath := fmt.Sprintf("%s/solution", dir)
	// Correctly pass the command and its arguments as separate strings
	cmd := exec.Command("g++", "-o", outFilePath, fmt.Sprintf("-std=%s", versionFlag), sourceFilePath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	e.logger.Infof("Running command [MsgID: %s]", messageID)
	cmdErr := cmd.Run()
	if cmdErr != nil {
		// Save stderr to a file
		e.logger.Errorf("Error during compilation. %s [MsgID: %s]", cmdErr.Error(), messageID)
		e.logger.Infof("Creating stderr file [MsgID: %s]", messageID)
		errPath := fmt.Sprintf("%s/%s", dir, CompileErrorFileName)
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
	if !utils.Contains(CPP_AVAILABLE_VERSION, version) {
		logger.Errorf("Invalid version supplied [MsgID: %s]", messageID)
		return nil, fmt.Errorf("invalid version supplied. got=%s, availabe=%s", version, CPP_AVAILABLE_VERSION)
	}
	return &CppExecutor{
		version: version,
		logger:  logger}, nil
}

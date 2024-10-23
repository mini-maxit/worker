package executor

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"

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

func (e *CppExecutor) ExecuteCommand(command string, commandConfig CommandConfig) *ExecutionResult {
	// Prepare command for execution
	cmd := exec.Command(command)

	stdout, err := os.OpenFile(commandConfig.StdoutPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not open stdout file. %s", err.Error()),
		}
	}
	cmd.Stdout = stdout
	defer utils.CloseFile(stdout)

	stderr, err := os.OpenFile(commandConfig.StderrPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("could not open stderr file. %s", err.Error()),
		}
	}
	cmd.Stderr = stderr
	defer utils.CloseFile(stderr)

	// Provide stdin if supplied
	if len(commandConfig.StdinPath) > 0 {
		stdin, err := os.Open(commandConfig.StdinPath)
		if err != nil {
			log.Printf("could not open stdin file. %s", err.Error())
			return &ExecutionResult{
				StatusCode: ErInternalError,
				Message:    fmt.Sprintf("could not open stdin file. %s", err.Error()),
			}
		}
		cmd.Stdin = stdin
		defer utils.CloseFile(stdin)
	}

	// Execute command
	err = cmd.Run()
	if err != nil {
		log.Printf("could not run the command. %s", err.Error())
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
		return &ExecutionResult{
			StatusCode: ErInternalError,
			Message:    fmt.Sprintf("command exited with unknown status code. %d", cmd.ProcessState.ExitCode()),
		}
	}

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
func (e *CppExecutor) Compile(sourceFilePath, dir string) (string, error) {
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
		return "", ErrInvalidVersion
	}
	outFilePath := fmt.Sprintf("%s/solution", dir)
	// Correctly pass the command and its arguments as separate strings
	cmd := exec.Command("g++", "-o", outFilePath, fmt.Sprintf("-std=%s", versionFlag), sourceFilePath)

	cmdErr := cmd.Run()
	if cmdErr != nil {
		// Save stderr to a file
		errPath := fmt.Sprintf("%s/compile-err.err", dir)
		file, err := os.Create(errPath)
		if err != nil {
			return "", err
		}
		_, err = file.Write([]byte(cmdErr.Error()))
		if err != nil {
			return "", err
		}
		err = file.Close()
		if err != nil {
			return "", err
		}
		log.Printf("Error during compilation. Saved error to %s", errPath)
		return errPath, cmdErr
	}
	log.Printf("Compiled %s to %s", sourceFilePath, outFilePath)
	return outFilePath, nil
}

func NewCppExecutor(version string) (*CppExecutor, error) {
	if !utils.Contains(CPP_AVAILABLE_VERSION, version) {
		return nil, fmt.Errorf("invalid version supplied. got=%s, availabe=%s", version, CPP_AVAILABLE_VERSION)
	}
	return &CppExecutor{version: version}, nil
}

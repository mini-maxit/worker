package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/utils"
)

type CommandConfig struct {
	StdinPath  string // Path to the file to be used as stdin
	StdoutPath string // Path to the file to be used as stdout
	// May be dropped in the future
	StderrPath    string // Path to the file to be used as stderr
	TimeLimit     int    // Time limit for the execution in milliseconds
	MemoryLimit   int    // Memory limit for the execution in kbytes
	ChrootDirPath string
	UseChroot     bool // Flag to signify if the solution will be run in chroot isolation
}

type Executor interface {
	ExecuteCommand(command, messageID string, commandConfig CommandConfig) *ExecutionResult
	RequiresCompilation() bool // Indicate whether the program should be compiled before execution
	Compile(filePath, dir, messageID string) (string, error)
	String() string
}

func ExitCodeToString(exitCode int) string {
	switch exitCode {
	case constants.ExitCodeSuccess:
		return "Success"
	case constants.ExitCodeTimeLimitExceeded:
		return "TimeLimitExceeded"
	case constants.ExitCodeMemoryLimitExceeded:
		return "MemoryLimitExceeded"
	case constants.ExitCodeInternalError:
		return "InternalError"
	default:
		return "Unknown"
	}
}

type ExecutionResult struct {
	ExitCode int
	Message  string
}

type IOConfig struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

func (er *ExecutionResult) String() string {
	var out bytes.Buffer

	out.WriteString("ExecutionResult{")
	out.WriteString(fmt.Sprintf("ExitCode: %d, ", er.ExitCode))
	out.WriteString("}")

	return out.String()
}

func restrictCommand(executableName string, commandConfig CommandConfig) *exec.Cmd {
	logger := logger.NewNamedLogger("executor")
	logger.Infof("Restricting command %s", executableName)

	timeLimitSecondsString := strconv.Itoa(commandConfig.TimeLimit)

	var args []string
	var executeCommand string
	if commandConfig.UseChroot {
		args = append(args, "chroot", commandConfig.ChrootDirPath)
		executeCommand = fmt.Sprintf("./%s", executableName)
	} else {
		executeCommand = fmt.Sprintf("%s/%s", commandConfig.ChrootDirPath, executableName)
	}

	args = append(args, "timeout", "--verbose", timeLimitSecondsString, executeCommand)

	return exec.Command(args[0], args[1:]...)
}

func copyExecutableToChrootAndRestric(executablePath string, commandConfig CommandConfig) (*exec.Cmd, string, error) {
	id := uuid.New()

	executableName := filepath.Base(executablePath)
	chrootExecName := fmt.Sprintf("%s_%s", id.String(), executableName)
	rootToChrootExecPath := fmt.Sprintf("%s/%s", commandConfig.ChrootDirPath, chrootExecName)
	// Copy the executable to the chroot
	err := utils.CopyFile(executablePath, rootToChrootExecPath)
	if err != nil {
		return nil, rootToChrootExecPath, err
	}

	// Change the permissions of the executable
	err = os.Chmod(rootToChrootExecPath, 0755)
	if err != nil {
		return nil, rootToChrootExecPath, err
	}

	// Restrict command to run in chroot with time limit
	restrictedCmd := restrictCommand(chrootExecName, commandConfig)

	return restrictedCmd, rootToChrootExecPath, nil
}

func setupIOFiles(stdinFilePath, stdoutFilePath, stderrFilePath string) (IOConfig, error) {
	ioConfig := IOConfig{}
	// Open stdout file
	stdout, err := os.OpenFile(stdoutFilePath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return IOConfig{}, fmt.Errorf("could not open stdout file. %s", err.Error())
	}

	ioConfig.Stdout = stdout

	// Open stderr file
	stderr, err := os.OpenFile(stderrFilePath, os.O_RDWR|os.O_CREATE|os.O_SYNC, 0755)
	if err != nil {
		return IOConfig{}, fmt.Errorf("could not open stderr file. %s", err.Error())
	}

	ioConfig.Stderr = stderr

	// Handle stdin if supplied
	if len(stdinFilePath) > 0 {
		// Open stdin
		stdin, err := os.OpenFile(stdinFilePath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return IOConfig{}, fmt.Errorf("could not open stdin file. %s", err.Error())
		}
		ioConfig.Stdin = stdin
	}

	return ioConfig, nil
}

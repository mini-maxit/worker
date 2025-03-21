package executor

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/mini-maxit/worker/internal/constants"
	"github.com/mini-maxit/worker/internal/logger"
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
	IsCompiled() bool // Indicate whether the program should be compiled before execution
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

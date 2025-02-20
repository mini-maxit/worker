package executor

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/mini-maxit/worker/logger"
)

type CommandConfig struct {
	StdinPath  string // Path to the file to be used as stdin
	StdoutPath string // Path to the file to be used as stdout
	// May be dropped in the future
	StderrPath  string // Path to the file to be used as stderr
	TimeLimit   int    // Time limit for the execution in milliseconds
	MemoryLimit int    // Memory limit for the execution in kbytes
}

type Executor interface {
	ExecuteCommand(command, messageID string, commandConfig CommandConfig) *ExecutionResult
	IsCompiled() bool // Indicate whether the program should be compiled before execution
	Compile(filePath, dir, messageID string) (string, error)
	String() string
}

type ExecutorStatusCode int

const (
	ErInternalError ExecutorStatusCode = iota
	ErSuccess
	ErSignalRecieved
	ErNetworkProhibited
	ErJailed
	ErTimeout
	ErMemoryLimitExceeded
)

const CompileErrorFileName = "compile-err.err"

const BaseChrootDir = "../tmp/chroot"

type ExecutionResult struct {
	StatusCode ExecutorStatusCode
	Message    string
}

func (ec ExecutorStatusCode) String() string {
	switch ec {
	case ErInternalError:
		return "InternalError"
	case ErSuccess:
		return "Success"
	case ErSignalRecieved:
		return "SignalRecieved"
	case ErNetworkProhibited:
		return "NetworkProhibited"
	case ErJailed:
		return "Jailed"
	case ErTimeout:
		return "Timeout"
	case ErMemoryLimitExceeded:
		return "MemoryLimitExceeded"
	default:
		return "Unknown"
	}
}

func (er *ExecutionResult) String() string {
	var out bytes.Buffer

	out.WriteString("ExecutionResult{")
	out.WriteString(fmt.Sprintf("StatusCode: %d, ", er.StatusCode))
	out.WriteString("}")

	return out.String()
}

func restrictCommand(executablePath string, timeLimit int) *exec.Cmd {
	logger := logger.NewNamedLogger("executor")
	logger.Infof("Restricting command %s", executablePath)

	executeCommand := fmt.Sprintf("./%s", executablePath)
	timeLimitSecondsString := strconv.Itoa(timeLimit)

	args := []string{
		"chroot",
		BaseChrootDir,
		"timeout",
		timeLimitSecondsString,
		executeCommand,
	}

	return exec.Command(args[0], args[1:]...)
}

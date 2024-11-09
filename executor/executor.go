package executor

import (
	"bytes"
	"fmt"
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
)

const CompileErrorFileName = "compile-err.err"

type ExecutionResult struct {
	StatusCode ExecutorStatusCode
	Message    string
}

func (er *ExecutionResult) String() string {
	var out bytes.Buffer

	out.WriteString("ExecutionResult{")
	out.WriteString(fmt.Sprintf("StatusCode: %d, ", er.StatusCode))
	out.WriteString("}")

	return out.String()
}

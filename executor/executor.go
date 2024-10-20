package executor

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/mini-maxit/worker/utils"
)

type CommandConfig struct {
	StdinSource string // Path to the file to be used as stdin
	StdoutPath  string // Path to the file to be used as stdout
	StderrPath  string // Path to the file to be used as stderr
}

type Executor interface {
	ExecuteCommand(command string, commandConfig CommandConfig) *ExecutionResult
	IsCompiled() bool // Indicate whether the program should be compiled before execution
	Compile(filePath string, dir string) (string, error)
	String() string
}

type DefaultExecutor struct {
	config *ExecutorConfig
}

type ExecutionResult struct {
	StatusCode int
	Message    string
}

func (er *ExecutionResult) String() string {
	var out bytes.Buffer

	out.WriteString("ExecutionResult{")
	out.WriteString(fmt.Sprintf("StatusCode: %d, ", er.StatusCode))
	out.WriteString("}")

	return out.String()
}

func (de *DefaultExecutor) ExecuteCommand(command string, commandConfig CommandConfig) *ExecutionResult {
	// Prepare command for execution
	cmd := exec.Command(command)
	dir, err := os.MkdirTemp(os.TempDir(), command)
	if err != nil {
		log.Fatalf("could not create temp dir. %s", err.Error())
	}
	log.Printf("Dir: %s", dir)

	stdout, err := os.CreateTemp(dir, "stdout-*")
	if err != nil {
		log.Fatalf("could not create temp file. %s", err.Error())
	}
	cmd.Stdout = stdout
	defer utils.CloseFile(stdout)

	stderr, err := os.CreateTemp(dir, "stderr-*")
	if err != nil {
		log.Fatalf("could not create temp file. %s", err.Error())
	}
	cmd.Stderr = stderr
	defer utils.CloseFile(stderr)

	// Provide stdin if supplied
	if len(commandConfig.StdinSource) > 0 {
		stdin, err := os.Open(commandConfig.StdinSource)
		if err != nil {
			log.Fatalf("could not open stdin file. %s", err.Error())
		}
		cmd.Stdin = stdin
		defer utils.CloseFile(stdin)
	}

	// Execute command
	err = cmd.Run()
	if err != nil {
		log.Fatalf("could not run the command. %s", err.Error())
	}

	// Read the output
	stdout.Seek(0, 0)
	bufferSize := 1000
	buffer := make([]byte, bufferSize)
	n, err := stdout.Read(buffer)
	log.Print("PROGRAM STDOUT:")
	for err == nil && n != 0 {
		fmt.Printf("%s", buffer)
		n, err = stdout.Read(buffer)
	}
	if err != nil {
		if err != io.EOF {
			log.Fatalf("error while reading output of command. %s", err.Error())
		}
	}
	log.Print("END PROGRAM STDOUT")

	executionResult := &ExecutionResult{
		StatusCode: cmd.ProcessState.ExitCode(),
	}
	return executionResult

}

func (de *DefaultExecutor) String() string {
	var out bytes.Buffer

	out.WriteString("DefaultExecutor{")
	out.WriteString("Config: " + de.config.String())
	out.WriteString("}")

	return out.String()

}

func NewDefaultExecutor(config *ExecutorConfig) *DefaultExecutor {
	return &DefaultExecutor{config: config}
}

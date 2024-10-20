package executor

import (
	"fmt"

	"github.com/mini-maxit/worker/utils"
)

var (
	PY_AVAILABLE_VERSION = []string{"3.11"}
)

type PyExecutor struct {
	version string
}

func (e *PyExecutor) ExecuteCommand(command string) *ExecutionResult {
	return &ExecutionResult{
		StatusCode: 0,
	}
}

func (e *PyExecutor) String() string {
	return "PyExecutor{}"
}

func NewPyExecutor(version string) (*PyExecutor, error) {
	if !utils.Contains(PY_AVAILABLE_VERSION, version) {
		return nil, fmt.Errorf("invalid error supplied. got=%s", version)
	}
	return &PyExecutor{version: version}, nil
}

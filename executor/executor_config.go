package executor

import (
	"bytes"
)

type ExecutorConfig struct {
	// Placeholder, maybe verifier will be stored here
}

const (
	ExitCodeTimeout = 124
)

// Creates config with default base values
func NewDefaultExecutorConfig() *ExecutorConfig {
	return &ExecutorConfig{}
}

func (ec *ExecutorConfig) String() string {
	var out bytes.Buffer

	out.WriteString("ExecutorConfig{")
	out.WriteString("}")

	return out.String()
}

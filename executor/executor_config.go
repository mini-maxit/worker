package executor

import (
	"bytes"
	"fmt"
	"time"
)

type ExecutorConfig struct {
	MemoryLimit int // memory limit for executing command in KB
	TimeLimit   int // time limit for executing command in seconds
}

// Creates config with default base values
func NewDefaultExecutorConfig() *ExecutorConfig {
	return &ExecutorConfig{
		TimeLimit:   int(time.Second * 20), // 20 seconds
		MemoryLimit: 1000 * 50,             // 50 KB
	}
}

func (ec *ExecutorConfig) String() string {
	var out bytes.Buffer

	out.WriteString("ExecutorConfig{")
	out.WriteString(fmt.Sprintf("MemoryLimit: %d, ", ec.MemoryLimit))
	out.WriteString(fmt.Sprintf("TimeLimit: %d, ", ec.TimeLimit))
	out.WriteString("}")

	return out.String()
}

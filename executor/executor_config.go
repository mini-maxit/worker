package executor

import (
	"bytes"
)

type Config struct {
	// Placeholder, maybe verifier will be stored here.
}

// Creates config with default base values.
func NewDefaultExecutorConfig() *Config {
	return &Config{}
}

func (ec *Config) String() string {
	var out bytes.Buffer

	out.WriteString("Config{")
	out.WriteString("}")

	return out.String()
}

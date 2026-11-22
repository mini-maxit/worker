package executor

import (
	"testing"

	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/stretchr/testify/assert"
)

func TestBuildHostConfigMemory(t *testing.T) {
	small := []messages.TestCase{{MemoryLimitKB: 64 * 1024}}
	big := []messages.TestCase{{MemoryLimitKB: 1024 * 1024}}

	// interpreted: test limit + 64 MB headroom, unchanged
	assert.Equal(t, int64(128*1024*1024), buildHostConfig(small, false).Memory)
	// compiled: raised to the compile floor
	assert.Equal(t, constants.MinCompileContainerMemoryKB*1024, buildHostConfig(small, true).Memory)
	// compiled with a limit above the floor: test limit + 20% headroom wins
	assert.Equal(t, int64((1024*1024+1024*1024/5)*1024), buildHostConfig(big, true).Memory)
}

package runtimescript_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/mini-maxit/worker/internal/runtimescript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The publish workflow tags images with `script-$(sha256sum run_tests.sh | cut -c1-12)`.
func TestImageTagMatchesWorkflow(t *testing.T) {
	script, err := os.ReadFile("run_tests.sh")
	require.NoError(t, err)

	assert.Equal(t, "script-"+fmt.Sprintf("%x", sha256.Sum256(script))[:12], runtimescript.ImageTag())
}

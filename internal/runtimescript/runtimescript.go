// Package runtimescript holds the test runner baked into every runtime image. Its hash tags those images,
// so a worker only runs images built with the script it was compiled against.
package runtimescript

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed run_tests.sh
var script []byte

// ImageTag is the runtime image tag; .github/workflows/runtime-images.yaml derives the same value.
func ImageTag() string {
	sum := sha256.Sum256(script)
	return "script-" + hex.EncodeToString(sum[:6])
}

package languages_test

import (
	"sort"
	"testing"

	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/stretchr/testify/assert"
)

func TestGetAllDockerImages(t *testing.T) {
	images := languages.GetAllDockerImages()
	sort.Strings(images)
	// all C++ versions share one image; each Python version has its own
	assert.Equal(t, []string{
		"ghcr.io/mini-maxit/runtime-cpp:latest",
		"ghcr.io/mini-maxit/runtime-python-3.10:latest",
		"ghcr.io/mini-maxit/runtime-python-3.11:latest",
		"ghcr.io/mini-maxit/runtime-python-3.12:latest",
	}, images)
}

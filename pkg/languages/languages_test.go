package languages_test

import (
	"testing"

	"github.com/mini-maxit/worker/internal/runtimescript"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllDockerImages(t *testing.T) {
	images, err := languages.GetAllDockerImages()
	require.NoError(t, err)

	var expected []string
	for lang, versions := range languages.LanguageVersionMap {
		for version := range versions {
			image, err := lang.GetDockerImage(version)
			require.NoError(t, err)
			assert.Regexp(t, ":"+runtimescript.ImageTag()+"$", image)
			expected = append(expected, image)
		}
	}
	assert.Subset(t, images, expected)
	// all C++ versions share one image; each Python version has its own
	assert.Len(t, images, 1+len(languages.LanguageVersionMap[languages.PYTHON]))
	seen := map[string]bool{}
	for _, image := range images {
		assert.False(t, seen[image], "duplicate image %s", image)
		seen[image] = true
	}
}

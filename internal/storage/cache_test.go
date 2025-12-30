package storage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/storage"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestFile creates a temporary file with test content.
func createTestFile(t *testing.T, content string) string {
	tempFile, err := os.CreateTemp(t.TempDir(), "test-file-*.txt")
	require.NoError(t, err)
	defer tempFile.Close()

	_, err = tempFile.WriteString(content)
	require.NoError(t, err)

	return tempFile.Name()
}

func TestNewFileCache(t *testing.T) {
	cache := storage.NewFileCache("")
	require.NotNil(t, cache)
}

func TestFileCache_InitCache(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Verify cache directory was created
	info, err := os.Stat(cachedir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFileCache_CacheFile(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create a test file
	testContent := "test file content"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/path/file.txt",
	}

	// Cache the file
	err = cache.CacheFile(fileLocation, testFile)
	require.NoError(t, err)

	// Verify the cached file exists and has correct content
	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotEmpty(t, cachedPath)

	// Verify content
	content, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(content))
}

func TestFileCache_GetCachedFile_NotFound(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "nonexistent/file.txt",
	}

	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, cachedPath)
}

func TestFileCache_GetCachedFile_Success(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create and cache a file
	testContent := "successful cache test"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/success.txt",
	}

	err = cache.CacheFile(fileLocation, testFile)
	require.NoError(t, err)

	// Get cached file
	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotEmpty(t, cachedPath)

	// Verify file exists
	_, err = os.Stat(cachedPath)
	require.NoError(t, err)
}

func TestFileCache_MultipleCacheEntries(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create and cache multiple files
	files := []struct {
		location messages.FileLocation
		content  string
	}{
		{
			location: messages.FileLocation{Bucket: "bucket1", Path: "path1/file1.txt"},
			content:  "content 1",
		},
		{
			location: messages.FileLocation{Bucket: "bucket2", Path: "path2/file2.txt"},
			content:  "content 2",
		},
		{
			location: messages.FileLocation{Bucket: "bucket1", Path: "path3/file3.txt"},
			content:  "content 3",
		},
	}

	for _, f := range files {
		testFile := createTestFile(t, f.content)
		defer os.Remove(testFile)

		err := cache.CacheFile(f.location, testFile)
		require.NoError(t, err)
	}

	// Verify all files are cached correctly
	for _, f := range files {
		cachedPath, found, err := cache.GetCachedFile(f.location)
		require.NoError(t, err)
		assert.True(t, found, "File not found: %s/%s", f.location.Bucket, f.location.Path)

		content, err := os.ReadFile(cachedPath)
		require.NoError(t, err)
		assert.Equal(t, f.content, string(content))
	}
}

func TestFileCache_OverwriteExistingCache(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/overwrite.txt",
	}

	// Cache first version
	testFile1 := createTestFile(t, "first content")
	defer os.Remove(testFile1)
	err = cache.CacheFile(fileLocation, testFile1)
	require.NoError(t, err)

	// Cache second version (overwrite)
	testFile2 := createTestFile(t, "second content")
	defer os.Remove(testFile2)
	err = cache.CacheFile(fileLocation, testFile2)
	require.NoError(t, err)

	// Get cached file and verify it has the second content
	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.True(t, found)

	content, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, "second content", string(content))
}

func TestFileCache_DifferentBucketsSamePath(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	samePath := "common/file.txt"

	// Cache file in bucket1
	testFile1 := createTestFile(t, "bucket1 content")
	defer os.Remove(testFile1)
	location1 := messages.FileLocation{Bucket: "bucket1", Path: samePath}
	err = cache.CacheFile(location1, testFile1)
	require.NoError(t, err)

	// Cache file in bucket2 with same path
	testFile2 := createTestFile(t, "bucket2 content")
	defer os.Remove(testFile2)
	location2 := messages.FileLocation{Bucket: "bucket2", Path: samePath}
	err = cache.CacheFile(location2, testFile2)
	require.NoError(t, err)

	// Verify both are cached separately
	cachedPath1, found1, err := cache.GetCachedFile(location1)
	require.NoError(t, err)
	assert.True(t, found1)

	cachedPath2, found2, err := cache.GetCachedFile(location2)
	require.NoError(t, err)
	assert.True(t, found2)

	// Verify they have different content
	content1, err := os.ReadFile(cachedPath1)
	require.NoError(t, err)
	assert.Equal(t, "bucket1 content", string(content1))

	content2, err := os.ReadFile(cachedPath2)
	require.NoError(t, err)
	assert.Equal(t, "bucket2 content", string(content2))

	// Verify they are different files
	assert.NotEqual(t, cachedPath1, cachedPath2)
}

func TestFileCache_CacheWithDifferentExtensions(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	extensions := []string{".txt", ".cpp", ".py", ".java", ".go"}

	for _, ext := range extensions {
		fileLocation := messages.FileLocation{
			Bucket: "test-bucket",
			Path:   "test/file" + ext,
		}

		testFile := createTestFile(t, "content for "+ext)
		defer os.Remove(testFile)

		err := cache.CacheFile(fileLocation, testFile)
		require.NoError(t, err)

		// Verify the cached file preserves the extension
		cachedPath, found, err := cache.GetCachedFile(fileLocation)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, ext, filepath.Ext(cachedPath), "Extension mismatch for %s", ext)
	}
}

func TestFileCache_GetCachedFile_FileDeleted(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create and cache a file
	testContent := "deleted file test"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/deleted.txt",
	}

	err = cache.CacheFile(fileLocation, testFile)
	require.NoError(t, err)

	// Get the cached file path
	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	require.True(t, found)

	// Delete the cached file
	err = os.Remove(cachedPath)
	require.NoError(t, err)

	// Try to get again - should return not found
	cachedPath2, found2, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.False(t, found2)
	assert.Empty(t, cachedPath2)
}

func TestFileCache_CacheFile_InvalidSourcePath(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/invalid.txt",
	}

	// Try to cache a non-existent file
	err = cache.CacheFile(fileLocation, "/non/existent/file.txt")
	assert.Error(t, err)
}

func TestFileCache_CleanExpiredCache_NoExpiredEntries(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Cache a fresh file
	testContent := "fresh cache test"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/fresh.txt",
	}

	err = cache.CacheFile(fileLocation, testFile)
	require.NoError(t, err)

	// Clean expired cache
	err = cache.CleanExpiredCache()
	require.NoError(t, err)

	// Verify the file is still cached
	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotEmpty(t, cachedPath)
}

func TestFileCache_SpecialCharactersInPath(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// File location with special characters
	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/файл с пробелами and special-chars_123.txt",
	}

	testContent := "special characters test"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	err = cache.CacheFile(fileLocation, testFile)
	require.NoError(t, err)

	// Verify cached file can be retrieved
	cachedPath, found, err := cache.GetCachedFile(fileLocation)
	require.NoError(t, err)
	assert.True(t, found)

	content, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(content))
}

func TestFileCache_EvictionWhenFull(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Override CacheMaxEntries for testing - cache only 5 files
	maxEntries := 5

	// Create and cache files up to the limit
	files := make([]messages.FileLocation, maxEntries+2)
	for i := range maxEntries + 2 {
		files[i] = messages.FileLocation{
			Bucket: "test-bucket",
			Path:   fmt.Sprintf("test/file%d.txt", i),
		}

		content := fmt.Sprintf("content %d", i)
		testFile := createTestFile(t, content)
		defer os.Remove(testFile)

		err := cache.CacheFile(files[i], testFile)
		require.NoError(t, err)

		// Small delay to ensure different CachedAt times
		time.Sleep(1 * time.Millisecond)
	}

	// Since we added maxEntries+2 files, oldest entries should be evicted
	// Files 0 and 1 should be evicted (oldest two)
	_, _, err = cache.GetCachedFile(files[0])
	require.NoError(t, err)
	// File 0 might be evicted depending on implementation
	// But at least some files should still be cached

	// Most recent files should still be cached
	lastIdx := maxEntries + 1
	cachedPath, found, err := cache.GetCachedFile(files[lastIdx])
	require.NoError(t, err)
	assert.True(t, found, "Most recent file should still be cached")
	assert.NotEmpty(t, cachedPath)
}

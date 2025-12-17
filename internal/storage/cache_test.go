package storage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/storage"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTaskVersion = "v1.0.0"

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
	taskVersion := testTaskVersion

	// Cache the file
	err = cache.CacheFile(fileLocation, taskVersion, testFile)
	require.NoError(t, err)

	// Verify the cached file exists and has correct content
	cachedPath, found, err := cache.GetCachedFile(fileLocation, taskVersion)
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

	cachedPath, found, err := cache.GetCachedFile(fileLocation, testTaskVersion)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, cachedPath)
}

func TestFileCache_GetCachedFile_VersionMismatch(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create and cache a file
	testContent := "test content"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/file.txt",
	}

	err = cache.CacheFile(fileLocation, testTaskVersion, testFile)
	require.NoError(t, err)

	// Try to get with different version
	cachedPath, found, err := cache.GetCachedFile(fileLocation, "v2.0.0")
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
	taskVersion := testTaskVersion

	err = cache.CacheFile(fileLocation, taskVersion, testFile)
	require.NoError(t, err)

	// Get cached file
	cachedPath, found, err := cache.GetCachedFile(fileLocation, taskVersion)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotEmpty(t, cachedPath)

	// Verify file exists
	_, err = os.Stat(cachedPath)
	require.NoError(t, err)
}

func TestFileCache_CleanExpiredCache(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create and cache a file
	testContent := "expired cache test"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/expired.txt",
	}
	taskVersion := testTaskVersion

	err = cache.CacheFile(fileLocation, taskVersion, testFile)
	require.NoError(t, err)

	// Manually update metadata to simulate expired cache
	metadataPath := filepath.Join(cachedir, constants.CacheMetadataFile)
	data, err := os.ReadFile(metadataPath)
	require.NoError(t, err)

	var metadata storage.CacheMetadata
	err = json.Unmarshal(data, &metadata)
	require.NoError(t, err)

	// Set all entries to be expired (more than 24 hours old)
	for key, entry := range metadata.Entries {
		entry.CachedAt = time.Now().Add(-25 * time.Hour)
		metadata.Entries[key] = entry
	}

	// Save modified metadata
	modifiedData, err := json.MarshalIndent(metadata, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(metadataPath, modifiedData, 0644)
	require.NoError(t, err)

	// Create new cache instance to reload metadata
	cache2 := storage.NewFileCache(cachedir)
	err = cache2.InitCache()
	require.NoError(t, err)

	// Try to get the file - should not be found as it was cleaned
	cachedPath, found, err := cache2.GetCachedFile(fileLocation, taskVersion)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, cachedPath)
}

func TestFileCache_MultipleCacheEntries(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	err := cache.InitCache()
	require.NoError(t, err)

	// Create and cache multiple files
	files := []struct {
		location messages.FileLocation
		version  string
		content  string
	}{
		{
			location: messages.FileLocation{Bucket: "bucket1", Path: "path1/file1.txt"},
			version:  testTaskVersion,
			content:  "content 1",
		},
		{
			location: messages.FileLocation{Bucket: "bucket2", Path: "path2/file2.txt"},
			version:  testTaskVersion,
			content:  "content 2",
		},
		{
			location: messages.FileLocation{Bucket: "bucket1", Path: "path3/file3.txt"},
			version:  "v2.0.0",
			content:  "content 3",
		},
	}

	for _, f := range files {
		testFile := createTestFile(t, f.content)
		defer os.Remove(testFile)

		err := cache.CacheFile(f.location, f.version, testFile)
		require.NoError(t, err)
	}

	// Verify all files are cached correctly
	for _, f := range files {
		cachedPath, found, err := cache.GetCachedFile(f.location, f.version)
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
	taskVersion := testTaskVersion

	// Cache first version
	testFile1 := createTestFile(t, "first content")
	defer os.Remove(testFile1)
	err = cache.CacheFile(fileLocation, taskVersion, testFile1)
	require.NoError(t, err)

	// Cache second version (overwrite)
	testFile2 := createTestFile(t, "second content")
	defer os.Remove(testFile2)
	err = cache.CacheFile(fileLocation, taskVersion, testFile2)
	require.NoError(t, err)

	// Get cached file and verify it has the second content
	cachedPath, found, err := cache.GetCachedFile(fileLocation, taskVersion)
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
	version := testTaskVersion

	// Cache file in bucket1
	testFile1 := createTestFile(t, "bucket1 content")
	defer os.Remove(testFile1)
	location1 := messages.FileLocation{Bucket: "bucket1", Path: samePath}
	err = cache.CacheFile(location1, version, testFile1)
	require.NoError(t, err)

	// Cache file in bucket2 with same path
	testFile2 := createTestFile(t, "bucket2 content")
	defer os.Remove(testFile2)
	location2 := messages.FileLocation{Bucket: "bucket2", Path: samePath}
	err = cache.CacheFile(location2, version, testFile2)
	require.NoError(t, err)

	// Verify both are cached separately
	cachedPath1, found1, err := cache.GetCachedFile(location1, version)
	require.NoError(t, err)
	assert.True(t, found1)

	cachedPath2, found2, err := cache.GetCachedFile(location2, version)
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

		err := cache.CacheFile(fileLocation, testTaskVersion, testFile)
		require.NoError(t, err)

		// Verify the cached file preserves the extension
		cachedPath, found, err := cache.GetCachedFile(fileLocation, testTaskVersion)
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
	taskVersion := testTaskVersion

	err = cache.CacheFile(fileLocation, taskVersion, testFile)
	require.NoError(t, err)

	// Get the cached file path
	cachedPath, found, err := cache.GetCachedFile(fileLocation, taskVersion)
	require.NoError(t, err)
	require.True(t, found)

	// Delete the cached file
	err = os.Remove(cachedPath)
	require.NoError(t, err)

	// Try to get again - should return not found
	cachedPath2, found2, err := cache.GetCachedFile(fileLocation, taskVersion)
	require.NoError(t, err)
	assert.False(t, found2)
	assert.Empty(t, cachedPath2)
}

func TestFileCache_InitCache_WithExistingMetadata(t *testing.T) {
	cachedir := t.TempDir()
	cache := storage.NewFileCache(cachedir)

	// First initialization
	err := cache.InitCache()
	require.NoError(t, err)

	// Cache a file
	testContent := "persistent metadata test"
	testFile := createTestFile(t, testContent)
	defer os.Remove(testFile)

	fileLocation := messages.FileLocation{
		Bucket: "test-bucket",
		Path:   "test/persistent.txt",
	}
	taskVersion := testTaskVersion

	err = cache.CacheFile(fileLocation, taskVersion, testFile)
	require.NoError(t, err)

	// Create a new cache instance (simulating restart)
	cache2 := storage.NewFileCache(cachedir)
	err = cache2.InitCache()
	require.NoError(t, err)

	// Verify the cached file is still available
	cachedPath, found, err := cache2.GetCachedFile(fileLocation, taskVersion)
	require.NoError(t, err)
	assert.True(t, found)
	assert.NotEmpty(t, cachedPath)

	content, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(content))
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
	err = cache.CacheFile(fileLocation, testTaskVersion, "/non/existent/file.txt")
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
	taskVersion := testTaskVersion

	err = cache.CacheFile(fileLocation, taskVersion, testFile)
	require.NoError(t, err)

	// Clean expired cache
	err = cache.CleanExpiredCache()
	require.NoError(t, err)

	// Verify the file is still cached
	cachedPath, found, err := cache.GetCachedFile(fileLocation, taskVersion)
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

	err = cache.CacheFile(fileLocation, testTaskVersion, testFile)
	require.NoError(t, err)

	// Verify cached file can be retrieved
	cachedPath, found, err := cache.GetCachedFile(fileLocation, testTaskVersion)
	require.NoError(t, err)
	assert.True(t, found)

	content, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(content))
}

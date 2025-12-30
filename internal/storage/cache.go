package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/utils"
	"go.uber.org/zap"
)

type CacheEntry struct {
	FilePath       string    `json:"file_path"`
	CachedAt       time.Time `json:"cached_at"`
	OriginalPath   string    `json:"original_path"`
	OriginalBucket string    `json:"original_bucket"`
}

type CacheMetadata struct {
	Entries map[string]CacheEntry `json:"entries"` // key is hash of bucket+path
}

type FileCache interface {
	GetCachedFile(fileLocation messages.FileLocation) (string, bool, error)
	CacheFile(fileLocation messages.FileLocation, sourcePath string) error
	CleanExpiredCache() error
	InitCache() error
}

type fileCache struct {
	logger       *zap.SugaredLogger
	cacheDirPath string
	ttl          time.Duration
	metadata     *CacheMetadata
}

func NewFileCache(cacheDirPath string) FileCache {
	logger := logger.NewNamedLogger("cache")
	return &fileCache{
		logger:       logger,
		cacheDirPath: cacheDirPath,
		ttl:          time.Duration(constants.CacheTTLHours) * time.Hour,
		metadata:     &CacheMetadata{Entries: make(map[string]CacheEntry)},
	}
}

// InitCache initializes the cache directory.
func (c *fileCache) InitCache() error {
	if err := os.MkdirAll(c.cacheDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := c.CleanExpiredCache(); err != nil {
		c.logger.Warnf("Failed to clean expired cache: %v", err)
	}

	return nil
}

func (c *fileCache) GetCachedFile(fileLocation messages.FileLocation) (string, bool, error) {
	key := c.generateKey(fileLocation)
	entry, exists := c.metadata.Entries[key]

	if !exists {
		return "", false, nil
	}

	// Check if cache is expired.
	if time.Since(entry.CachedAt) > c.ttl {
		c.logger.Debugf("Cache expired for %s", fileLocation.Path)
		delete(c.metadata.Entries, key)
		return "", false, nil
	}

	// Check if file still exists.
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		c.logger.Debugf("Cached file no longer exists: %s", entry.FilePath)
		delete(c.metadata.Entries, key)
		return "", false, nil
	}

	c.logger.Debugf("Cache hit for %s", fileLocation.Path)
	return entry.FilePath, true, nil
}

func (c *fileCache) CacheFile(fileLocation messages.FileLocation, sourcePath string) error {
	key := c.generateKey(fileLocation)

	// Ensure cache directory exists.
	if err := os.MkdirAll(c.cacheDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Evict oldest entries if cache is full and this is a new entry
	if _, exists := c.metadata.Entries[key]; !exists {
		if len(c.metadata.Entries) >= constants.CacheMaxEntries {
			c.evictOldestEntry()
		}
	}

	// Generate a unique cache file path.
	cacheFileName := c.generateCacheFileName(fileLocation)
	cacheFilePath := filepath.Join(c.cacheDirPath, cacheFileName)

	// Copy the file to cache.
	if err := utils.CopyFile(sourcePath, cacheFilePath); err != nil {
		return fmt.Errorf("failed to copy file to cache: %w", err)
	}

	// Update metadata.
	c.metadata.Entries[key] = CacheEntry{
		FilePath:       cacheFilePath,
		CachedAt:       time.Now(),
		OriginalPath:   fileLocation.Path,
		OriginalBucket: fileLocation.Bucket,
	}

	c.logger.Debugf("Cached file %s", fileLocation.Path)
	return nil
}

// CleanExpiredCache removes expired cache entries and orphaned files.
func (c *fileCache) CleanExpiredCache() error {
	now := time.Now()
	toDelete := []string{}

	for key, entry := range c.metadata.Entries {
		if now.Sub(entry.CachedAt) > c.ttl {
			toDelete = append(toDelete, key)
			if err := os.Remove(entry.FilePath); err != nil && !os.IsNotExist(err) {
				c.logger.Warnf("Failed to remove expired cache file %s: %v", entry.FilePath, err)
			}
		}
	}

	for _, key := range toDelete {
		delete(c.metadata.Entries, key)
	}

	if len(toDelete) > 0 {
		c.logger.Infof("Cleaned %d expired cache entries", len(toDelete))
	}

	return nil
}

func (c *fileCache) evictOldestEntry() {
	if len(c.metadata.Entries) == 0 {
		return
	}

	// Find the oldest entry
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.metadata.Entries {
		if first || entry.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CachedAt
			first = false
		}
	}

	// Remove the oldest entry
	if entry, exists := c.metadata.Entries[oldestKey]; exists {
		if err := os.Remove(entry.FilePath); err != nil && !os.IsNotExist(err) {
			c.logger.Warnf("Failed to remove evicted cache file %s: %v", entry.FilePath, err)
		}
		delete(c.metadata.Entries, oldestKey)
		c.logger.Debugf("Evicted oldest cache entry: %s", entry.OriginalPath)
	}
}

func (c *fileCache) generateKey(fileLocation messages.FileLocation) string {
	data := fmt.Sprintf("%s:%s", fileLocation.Bucket, fileLocation.Path)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (c *fileCache) generateCacheFileName(fileLocation messages.FileLocation) string {
	key := c.generateKey(fileLocation)
	ext := filepath.Ext(fileLocation.Path)
	return fmt.Sprintf("%s%s", key, ext)
}

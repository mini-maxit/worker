package utils

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/kballard/go-shellquote"
	"github.com/mini-maxit/worker/pkg/constants"
)

// Attemts to close the file, and panics if something goes wrong.
func CloseFile(file *os.File) {
	err := file.Close()
	if err != nil {
		// Check if the error is a PathError
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			log.Panicf("error during closing file %s. %s", pathErr.Path, pathErr.Error())
		}
		log.Panicf("unexpected error during closing file: %s", err.Error())
	}
}

// CopyFile copies a file from src to dst. It returns an error if any occurs during the copy.
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return err
	}

	err = destinationFile.Sync()
	if err != nil {
		return err
	}

	return nil
}

// moveFile tries os.Rename and falls back to copy+delete on EXDEV.
func MoveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) {
		return copyAndRemove(src, dst)
	}
	return err
}

// copyAndRemove copies src to dst and removes src. Used as a fallback for moveFile on EXDEV.
func copyAndRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// ensure data is flushed
	if err := out.Sync(); err != nil {
		return err
	}
	// delete the original
	return os.Remove(src)
}

// ValidateFilename checks if a filename contains only safe characters.
// Returns an error if the filename contains shell metacharacters or path separators
// that could be used for command injection or directory traversal attacks.
// Allowed characters: alphanumeric (a-z, A-Z, 0-9), dots (.), underscores (_), and hyphens (-).
func ValidateFilename(filename string) error {
	if filename == "" {
		return errors.New("filename cannot be empty")
	}

	// Check for path separators
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return errors.New("filename cannot contain path separators")
	}

	// Check for dangerous patterns
	if filename == "." || filename == ".." {
		return errors.New("filename cannot be a dot or double-dot")
	}

	// Only allow alphanumeric characters, dots, underscores, and hyphens
	// This prevents shell metacharacters like: ; & | $ ` ( ) < > [ ] { } ' " \ * ? # ~ ! space
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validPattern.MatchString(filename) {
		return errors.New("filename contains invalid characters")
	}

	return nil
}

func ShellQuoteSlice(strs []string) string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = shellquote.Join(s)
	}
	return strings.Join(quoted, " ")
}

// attempts to remove dir and optionaly its content. Can ignore error, for example if folder does not exist.
func RemoveIO(dir string, recursive, ignoreError bool) error {
	var err error
	if recursive {
		err = os.RemoveAll(dir)
	} else {
		err = os.Remove(dir)
	}

	if ignoreError {
		return nil
	}
	return err
}

// writeToTarArchive writes a file or directory to a tar archive.
func writeToTarArchive(tarWriter *tar.Writer, path, name string, info os.FileInfo) error {
	// Create tar header.
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	header.Gid = constants.RunnerGID
	header.Uid = constants.RunnerUID
	header.Gname = constants.RunnerName
	header.Uname = constants.RunnerName

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// If it's a file, write its contents.
	if !info.IsDir() {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		if _, err := io.Copy(tarWriter, file); err != nil {
			return err
		}
	}

	return nil
}

// CreateTarArchiveWithBaseFiltered works like CreateTarArchiveWithBase but skips any top-level entries
// whose names are present in excludeTopLevel (e.g. []string{"output"}).
// shouldExcludeFromTar checks if a path should be excluded from the tar archive.
func shouldExcludeFromTar(relPath string, excludeSet map[string]struct{}, isDir bool) (bool, error) {
	if relPath == "." {
		return false, nil
	}

	parts := strings.Split(relPath, string(os.PathSeparator))
	if len(parts) == 0 {
		return false, nil
	}

	if _, found := excludeSet[parts[0]]; found {
		if isDir {
			return true, filepath.SkipDir
		}
		return true, nil
	}

	return false, nil
}

func CreateTarArchiveWithBaseFiltered(srcPath string, excludeTopLevel []string) (io.ReadCloser, error) {
	pipeReader, pipeWriter := io.Pipe()

	go func() {
		tarWriter := tar.NewWriter(pipeWriter)
		defer func() { _ = tarWriter.Close() }()
		defer func() { _ = pipeWriter.Close() }()

		excludeSet := make(map[string]struct{}, len(excludeTopLevel))
		for _, e := range excludeTopLevel {
			excludeSet[e] = struct{}{}
		}

		parentPath := filepath.Dir(srcPath)

		err := filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Path relative to the parent keeps the base directory in the archive.
			relPathWithBase, err := filepath.Rel(parentPath, path)
			if err != nil {
				return err
			}

			// Path relative to srcPath is used for top-level filtering (children of srcPath).
			relPath, err := filepath.Rel(srcPath, path)
			if err != nil {
				return err
			}

			// Check if this path should be excluded.
			should, skipErr := shouldExcludeFromTar(relPath, excludeSet, info.IsDir())
			if should {
				return skipErr
			}

			return writeToTarArchive(tarWriter, path, relPathWithBase, info)
		})

		if err != nil {
			pipeWriter.CloseWithError(err)
		}
	}()

	return pipeReader, nil
}

func safeArchiveTarget(absDstPath, entryName string) (string, error) {
	target := filepath.Join(absDstPath, entryName)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTarget, absDstPath+string(os.PathSeparator)) && absTarget != absDstPath {
		return "", errors.New("tar archive entry would escape destination directory: " + entryName)
	}
	return target, nil
}

func materializeTarEntry(tarReader *tar.Reader, header *tar.Header, target string) error {
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(header.Mode))
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		safeMode := os.FileMode(header.Mode) & 0666 // Remove execute bits
		file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, safeMode)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, tarReader); err != nil {
			CloseFile(file)
			return err
		}
		CloseFile(file)
		return nil
	case tar.TypeSymlink, tar.TypeLink:
		return errors.New("symlinks and hardlinks are not allowed in archives")
	default:
		return nil
	}
}

func isAllowedDirectory(header *tar.Header, allowedSet map[string]struct{}) bool {
	parts := strings.Split(header.Name, string(os.PathSeparator))
	if len(parts) == 0 {
		return false
	}

	var baseDir string
	if len(parts) > 1 {
		baseDir = parts[1] // Skip the package directory itself
	} else {
		baseDir = parts[0]
	}

	_, allowed := allowedSet[baseDir]
	return allowed
}

func validatePathDepth(entryPath string) error {
	const maxDepth = 1

	parts := strings.Split(entryPath, string(os.PathSeparator))
	if len(parts) > maxDepth+1 {
		return fmt.Errorf("subdirectories not allowed in archives: %s", entryPath)
	}

	return nil
}

// validateTarEntrySize validates that a tar entry doesn't exceed maximum file size.
func validateTarEntrySize(header *tar.Header, maxFileSize int64) error {
	if header.Typeflag == tar.TypeReg && header.Size > maxFileSize {
		return fmt.Errorf("file %s exceeds maximum size of %d bytes (size: %d)", header.Name, maxFileSize, header.Size)
	}
	return nil
}

// validateAndTrackFileCount validates per-directory file count and updates the tracking map.
func validateAndTrackFileCount(header *tar.Header, dirFileCount map[string]int, maxFilesInDir int) error {
	if header.Typeflag != tar.TypeReg {
		return nil
	}

	// Extract the allowed directory name (second path component)
	parts := strings.Split(header.Name, string(os.PathSeparator))
	if len(parts) < 2 {
		return fmt.Errorf("invalid file path structure: %s", header.Name)
	}
	dirName := parts[1]

	dirFileCount[dirName]++
	if dirFileCount[dirName] > maxFilesInDir {
		return fmt.Errorf("directory %s exceeds maximum file count of %d", dirName, maxFilesInDir)
	}
	return nil
}

func ExtractTarArchiveFiltered(
	reader io.Reader,
	dstPath string,
	allowedDirs []string,
	maxFileSize int64,
	maxFilesInDir int,
) error {
	tarReader := tar.NewReader(reader)

	absDstPath, err := filepath.Abs(dstPath)
	if err != nil {
		return err
	}

	allowedSet := make(map[string]struct{}, len(allowedDirs))
	for _, dir := range allowedDirs {
		allowedSet[dir] = struct{}{}
	}

	// Track files per allowed directory to enforce per-directory limits
	dirFileCount := make(map[string]int, len(allowedDirs))

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		if !isAllowedDirectory(header, allowedSet) {
			continue
		}

		if err := validateTarEntrySize(header, maxFileSize); err != nil {
			return err
		}

		if err := validatePathDepth(header.Name); err != nil {
			return err
		}

		if err := validateAndTrackFileCount(header, dirFileCount, maxFilesInDir); err != nil {
			return err
		}

		target, err := safeArchiveTarget(absDstPath, header.Name)
		if err != nil {
			return err
		}

		if err := materializeTarEntry(tarReader, header, target); err != nil {
			return err
		}
	}
}

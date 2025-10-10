package utils

import (
	"errors"
	"io"
	"log"
	"os"
	"syscall"
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

// Checks if given elements is contained in given array.
func Contains[V string](array []V, value V) bool {
	for _, el := range array {
		if el == value {
			return true
		}
	}

	return false
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

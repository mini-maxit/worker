package utils

import (
	"log"
	"os"
)

// Attemts to close the file, and panics if something goes wrong
func CloseFile(file *os.File) {
	err := file.Close()
	if err != nil {
		err := err.(*os.PathError)
		log.Panicf("error during closing file %s. %s", err.Path, err.Error())
	}
}

// Checks if given elements is contained in given array
func Contains[V string](array []V, value V) bool {
	for _, el := range array {
		if el == value {
			return true
		}
	}

	return false
}

// attempts to remove dir and optionaly its content. Can ignore error, for example if folder does not exist
func RemoveIO(dir string, recursive, ignore_error bool) error {
	var err error
	if recursive {
		err = os.RemoveAll(dir)
	} else {
		err = os.Remove(dir)
	}

	if ignore_error {
		return nil
	}
	return err
}

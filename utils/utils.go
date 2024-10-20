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

// Helper function to log errors
func CheckError(err error, msg string) {
	if err != nil {
		log.Printf("%s: %s", msg, err)
	}
}

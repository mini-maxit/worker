package errors

import "errors"

// Error messages.
var (
	ErrFailedToUnmarshalTaskMessage = errors.New("failed to unmarshal task message")
	ErrFailedToHandleTaskPackage    = errors.New("failed to handle task package")
	ErrFailedToParseLanguageType    = errors.New("failed to parse language type")
	ErrFailedToGetSolutionFileName  = errors.New("failed to get solution file name")
	ErrDoesNotRequireCompilation    = errors.New("does not require compilation")
	ErrUnknownFileType              = errors.New("unknown file type")
	ErrInvalidLanguageType          = errors.New("invalid language type")
	ErrInvalidVersion               = errors.New("invalid version supplied")
	ErrFailedToStoreSolution        = errors.New("failed to store the solution result")
	ErrFailedToGetFreeWorker        = errors.New("failed to get free worker")
	ErrUnknownMessageType           = errors.New("unknown message type")
	ErrInvalidFilePath              = errors.New("invalid file path")
	ErrEmptyInputDirectory          = errors.New("empty input directory")
	ErrContainerTimeout             = errors.New("container runtime timed out")
	ErrContainerFailed              = errors.New("container failed to execute")
	ErrInputOutputMismatch          = errors.New("input output mismatch")
	ErrSubmissionFileLocationEmpty  = errors.New("submission file location is empty")
)

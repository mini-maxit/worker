package errors

import "errors"

// Error messages
var (
	ErrUnknownFileType              = errors.New("unknown file type")
	ErrInvalidLanguageType          = errors.New("invalid language type")
	ErrInvalidVersion               = errors.New("invalid version supplied")
	ErrFailedToStoreSolution        = errors.New("failed to store the solution result")
	ErrInvalidQueueMessageType      = errors.New("invalid queue message type")
	ErrFailedToUnmarshalTaskMessage = errors.New("failed to unmarshal task message")
	ErrFailedToHandleTaskPackage    = errors.New("failed to handle task package")
	ErrFailedToParseLanguageType    = errors.New("failed to parse language type")
	ErrFailedToGetSolutionFileName  = errors.New("failed to get solution file name")
	ErrMaxWorkersReached            = errors.New("maximum number of workers reached")
	ErrWorkerNotFound               = errors.New("worker not found")
)

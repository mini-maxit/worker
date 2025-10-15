package errors

import "errors"

// Error messages.
var (
	ErrInvalidLanguageType         = errors.New("invalid language type")
	ErrInvalidVersion              = errors.New("invalid version supplied")
	ErrFailedToGetFreeWorker       = errors.New("failed to get free worker")
	ErrUnknownMessageType          = errors.New("unknown message type")
	ErrContainerTimeout            = errors.New("container runtime timed out")
	ErrContainerFailed             = errors.New("container failed to execute")
	ErrSubmissionFileLocationEmpty = errors.New("submission file location is empty")
)

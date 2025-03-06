package errors

import "errors"

// Error messages
var (
	ErrUnknownFileType       = errors.New("unknown file type")
	ErrInvalidLanguageType   = errors.New("invalid language type")
	ErrInvalidVersion        = errors.New("invalid version supplied")
	ErrFailedToStoreSolution = errors.New("failed to store the solution result")
)

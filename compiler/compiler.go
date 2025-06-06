package compiler

import (
	"github.com/mini-maxit/worker/internal/errors"
	"github.com/mini-maxit/worker/internal/languages"
)

type Compiler interface {
	RequiresCompilation() bool
	Compile(filePath, dir, messageID string) (string, error)
}

func Initialize(
	languageType languages.LanguageType,
	languageVersion,
	messageID string,
) (Compiler, error) {
	switch languageType {
	case languages.CPP:
		return NewCppCompiler(languageVersion, messageID)
	default:
		return nil, errors.ErrInvalidLanguageType
	}
}

package compiler

import (
	customErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
)

type Compiler interface {
	CompileSolutionIfNeeded(langType languages.LanguageType, langVersion, sourceFilePath, outFilePath, compErrFilePath, messageID string) error
}

type compiler struct {
}

func NewCompiler() Compiler {
	return &compiler{}
}

// LanguageCompiler is a language-specific compiler invoked internally.
type LanguageCompiler interface {
	RequiresCompilation() bool
	Compile(sourceFilePath, outFilePath, compErrFilePath, messageID string) error
}

func initializeSolutionCompiler(langType languages.LanguageType, langVersion string, messageID string) (LanguageCompiler, error) {
	switch langType {
	case languages.CPP:
		return NewCppCompiler(langVersion, messageID)
	default:
		return nil, customErr.ErrInvalidLanguageType
	}
}

// get proper compiler and compile if needed
func (c *compiler) CompileSolutionIfNeeded(langType languages.LanguageType, langVersion, sourceFilePath, outFilePath, compErrFilePath, messageID string) error {
	compiler, err := initializeSolutionCompiler(langType, langVersion, messageID)
	if err != nil {
		return err
	}

	if compiler.RequiresCompilation() {
		return compiler.Compile(sourceFilePath, outFilePath, compErrFilePath, messageID)
	}
	return nil
}
package compiler

import (
	customErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
)

type Compiler interface {
	CompileSolutionIfNeeded(
		langType languages.LanguageType,
		langVersion string,
		sourceFilePath string,
		outFilePath string,
		compErrFilePath string,
		messageID string,
	) error
}

type compiler struct {
}

func NewCompiler() Compiler {
	return &compiler{}
}

// LanguageCompiler is a language-specific compiler invoked internally.
type LanguageCompiler interface {
	Compile(sourceFilePath, outFilePath, compErrFilePath, messageID string) error
}

func initializeSolutionCompiler(
	langType languages.LanguageType,
	langVersion string,
	messageID string,
) (LanguageCompiler, error) {
	switch langType {
	case languages.CPP:
		return NewCppCompiler(langVersion, messageID)
	case languages.PYTHON:
		return nil, customErr.ErrInvalidLanguageType // Make linter happy
	default:
		return nil, customErr.ErrInvalidLanguageType
	}
}

// get proper compiler and compile if needed.
func (c *compiler) CompileSolutionIfNeeded(
	langType languages.LanguageType,
	langVersion,
	sourceFilePath,
	execFilePath,
	compErrFilePath,
	messageID string,
) error {
	if langType.IsScriptingLanguage() {
		return nil
	}

	compiler, err := initializeSolutionCompiler(langType, langVersion, messageID)
	if err != nil {
		return err
	}

	return compiler.Compile(sourceFilePath, execFilePath, compErrFilePath, messageID)
}

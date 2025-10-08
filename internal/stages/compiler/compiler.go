package compiler

type Compiler interface {
	RequiresCompilation() bool
	Compile(filePath, dir, messageID string) (string, error)
}

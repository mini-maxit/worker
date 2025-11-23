package compiler_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"errors"

	. "github.com/mini-maxit/worker/internal/stages/compiler"
	pkgErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/tests"
)

func TestNewCompiler(t *testing.T) {
	c := NewCompiler()
	if c == nil {
		t.Fatalf("NewCompiler returned nil")
	}
}

func TestCompileSolutionIfNeeded_Success(t *testing.T) {
	dir := t.TempDir()
	src := tests.WriteFile(t, dir, "main.cpp", `

        #include <iostream>

        int main() {
            std::cout << "hello" << std::endl;
            return 0;
        }
    `)

	out := filepath.Join(dir, "outbin")
	compErr := filepath.Join(dir, "compile.err")

	c := NewCompiler()

	if err := c.CompileSolutionIfNeeded(languages.CPP, "17", src, out, compErr, "msg-id"); err != nil {
		t.Fatalf("expected compile to succeed, got: %v", err)
	}

	if info, err := os.Stat(out); err != nil {
		t.Fatalf("expected output binary to exist: %v", err)
	} else if info.IsDir() {
		t.Fatalf("expected output to be a file, got dir")
	}

	// run the produced binary to ensure it runs and returns 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, out)
	if outb, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running compiled binary failed: %v, output: %s", err, string(outb))
	}
}

func TestCompileSolutionIfNeeded_InvalidLanguage(t *testing.T) {
	dir := t.TempDir()
	src := tests.WriteFile(t, dir, "main.txt", `dummy`)
	out := filepath.Join(dir, "out")
	compErr := filepath.Join(dir, "compile.err")

	c := NewCompiler()

	// use an invalid language value (0)
	err := c.CompileSolutionIfNeeded(languages.LanguageType(0), "", src, out, compErr, "msg-id")
	if err == nil {
		t.Fatalf("expected error for invalid language")
	}
	if !errors.Is(err, pkgErr.ErrInvalidLanguageType) {
		t.Fatalf("expected ErrInvalidLanguageType, got: %v", err)
	}
}

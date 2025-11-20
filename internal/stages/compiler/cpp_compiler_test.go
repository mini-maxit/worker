package compiler_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"errors"

	. "github.com/mini-maxit/worker/internal/stages/compiler"
	pkgErr "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/tests"
)

func TestRequiresCompilation(t *testing.T) {
	c, err := NewCppCompiler("17", "msg-test")
	if err != nil {
		t.Fatalf("NewCppCompiler returned error: %v", err)
	}
	if !c.RequiresCompilation() {
		t.Fatalf("expected RequiresCompilation to be true")
	}
}

func TestCompileSuccess(t *testing.T) {
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

	c, err := NewCppCompiler("17", "msg-test")
	if err != nil {
		t.Fatalf("NewCppCompiler returned error: %v", err)
	}

	if err := c.Compile(src, out, compErr, "msg-id"); err != nil {
		t.Fatalf("expected compile to succeed, got: %v", err)
	}

	// binary should exist and be executable
	if info, err := os.Stat(out); err != nil {
		t.Fatalf("expected output binary to exist: %v", err)
	} else if info.IsDir() {
		t.Fatalf("expected output to be a file, got dir")
	}

	// run the produced binary to ensure it runs and returns 0
	cmd := exec.Command(out)
	if outb, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("running compiled binary failed: %v, output: %s", err, string(outb))
	}
}

func TestCompileFailureProducesErrorFile(t *testing.T) {
	dir := t.TempDir()
	// intentionally broken C++ source
	src := tests.WriteFile(t, dir, "bad.cpp", `

		#include <iostream>

		int main() {
			this is not valid C++
		}
	`)

	out := filepath.Join(dir, "outbad")
	compErr := filepath.Join(dir, "compile.err")

	c, err := NewCppCompiler("17", "msg-id")
	if err != nil {
		t.Fatalf("NewCppCompiler returned error: %v", err)
	}

	err = c.Compile(src, out, compErr, "msg-id")
	if err == nil {
		t.Fatalf("expected compile to fail for broken source")
	}
	if !errors.Is(err, pkgErr.ErrCompilationFailed) {
		t.Fatalf("expected ErrCompilationFailed, got: %v", err)
	}

	// error file should exist and contain some data
	info, statErr := os.Stat(compErr)
	if statErr != nil {
		t.Fatalf("expected compile.err to exist, stat error: %v", statErr)
	}
	if info.Size() == 0 {
		t.Fatalf("expected compile.err to contain compiler stderr")
	}
}

func TestNewCppCompilerIntegration(t *testing.T) {
	// NewCppCompiler uses languages.GetVersionFlag; pass valid version '17'
	cc, err := NewCppCompiler("17", "msg-test")
	if err != nil {
		t.Fatalf("NewCppCompiler returned error: %v", err)
	}

	dir := t.TempDir()
	src := tests.WriteFile(t, dir, "main2.cpp", `
	#include <cstdio>

		int main() {
			printf("ok\n");
			return 0;
		}
	`)
	out := filepath.Join(dir, "out2")
	compErr := filepath.Join(dir, "compile2.err")

	if err := cc.Compile(src, out, compErr, "msg-test"); err != nil {
		t.Fatalf("expected compile to succeed using NewCppCompiler, got: %v", err)
	}
}

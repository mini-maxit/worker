//go:build integration

package executor_test

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mini-maxit/worker/internal/docker"
	exec "github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/internal/stages/packager"
	"github.com/mini-maxit/worker/internal/stages/verifier"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	"github.com/mini-maxit/worker/pkg/solution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each of the three children stays within the 64 MB per-process limit; together they need 150 MB.
// The program prints how many of them could hold their memory at the same time.
const forkingSolution = `
#include <csignal>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <sys/wait.h>
#include <unistd.h>
int main() {
  pid_t pids[3];
  for (int i = 0; i < 3; i++) {
    int fd[2];
    if (pipe(fd) != 0) return 2;
    pids[i] = fork();
    if (pids[i] < 0) return 3;
    if (pids[i] == 0) {
      size_t n = 50u << 20;
      char *p = (char *)malloc(n);
      if (!p) _exit(1);
      memset(p, 1, n);
      if (write(fd[1], p, 1) != 1) _exit(1);
      pause();
      _exit(0);
    }
    close(fd[1]);
    char c;
    if (read(fd[0], &c, 1) < 0) return 4; // 0 means the child died
    close(fd[0]);
  }
  sleep(1);
  int alive = 0;
  for (pid_t pid : pids)
    if (waitpid(pid, nullptr, WNOHANG) == 0) {
      alive++;
      kill(pid, SIGKILL);
    }
  printf("%d\n", alive);
}
`

// Prints the limits the kernel enforces on the container while the tests run.
const sandboxProbe = `
#include <fstream>
#include <iostream>
#include <string>
int main() {
  for (const char *name : {"memory.max", "memory.swap.max", "pids.max", "cpu.max"}) {
    std::ifstream file(std::string("/sys/fs/cgroup/") + name);
    std::string value;
    std::getline(file, value);
    std::cout << name << "=" << value << "\n";
  }
  std::ifstream status("/proc/self/status");
  for (std::string line; std::getline(status, line);)
    if (line.rfind("CapEff", 0) == 0 || line.rfind("NoNewPrivs", 0) == 0) std::cout << line << "\n";
}
`

const sandboxLimits = `memory.max=134217728
memory.swap.max=0
pids.max=8
cpu.max=100000 100000
CapEff: 0000000000000000
NoNewPrivs: 1`

func allocating(megabytes int) string {
	return fmt.Sprintf(`
#include <cstdio>
#include <vector>
int main() {
  std::vector<char> v(%dull << 20, 1);
  printf("%%d\n", v.back());
}
`, megabytes)
}

// TestMemoryLimitsOnDocker runs compiled solutions through the real executor and verifier.
func TestMemoryLimitsOnDocker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	buildCppImage(t)
	dockerClient, err := docker.NewDockerClient()
	require.NoError(t, err)

	cases := []struct {
		name     string
		source   string
		memoryKB []int64 // one test case per limit
		expected string
		want     solution.TestCaseStatus
	}{
		{"accepted", `#include <cstdio>
int main() { int a, b; if (scanf("%d %d", &a, &b) == 2) printf("%d\n", a + b); }`,
			[]int64{64 * 1024}, "5", solution.TestCasePassed},
		{"wrong answer", `#include <cstdio>
int main() { puts("4"); }`, []int64{64 * 1024}, "5", solution.OutputDifference},
		{"time limit", `int main() { volatile int x = 0; for (;;) x = x + 1; }`,
			[]int64{64 * 1024}, "5", solution.TimeLimitExceeded},
		{"non-zero exit", `int main() { return 3; }`, []int64{64 * 1024}, "5", solution.NonZeroExitCode},
		{"memory limit", allocating(100), []int64{64 * 1024}, "1", solution.MemoryLimitExceeded},
		{"just under the limit", allocating(50), []int64{64 * 1024}, "1", solution.TestCasePassed},
		{"static array over the limit", `char a[200 << 20]; int main(int c, char **) { a[c] = 1; return a[c + 1]; }`,
			[]int64{64 * 1024}, "", solution.NonZeroExitCode},
		{"large output", `#include <cstdio>
int main() { for (int i = 0; i < 1000000; i++) puts("1234567"); }`,
			[]int64{64 * 1024}, repeatLine("1234567", 1000000), solution.TestCasePassed},
		{"heavy compile", `#include <bits/stdc++.h>
int main() { std::regex r("a+"); std::cout << std::regex_match("aaa", r) << "\n"; }`,
			[]int64{64 * 1024}, "1", solution.TestCasePassed},
		{"limit above the compile floor", allocating(600), []int64{700 * 1024}, "1", solution.TestCasePassed},
		{"several test cases", allocating(50), []int64{64 * 1024, 128 * 1024}, "1", solution.TestCasePassed},
		{"sandbox during tests", sandboxProbe, []int64{64 * 1024}, sandboxLimits, solution.TestCasePassed},
		{"forking over the limit", forkingSolution, []int64{64 * 1024}, "3", solution.OutputDifference},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, testCases := preparePackage(t, c.source, c.memoryKB, c.expected)

			require.NoError(t, exec.NewExecutor(dockerClient, 60).ExecuteCommand(exec.CommandConfig{
				MessageID:         filepath.Base(dir.PackageDirPath),
				DirConfig:         dir,
				LanguageType:      languages.CPP,
				LanguageVersion:   "17",
				TestCases:         testCases,
				SourceFilePath:    dir.UserSolutionPath,
				RequiresCompiling: true,
			}))
			require.Empty(t, readFile(t, dir.CompileErrFilePath))

			result := verifier.NewVerifier([]string{constants.DefaultVerifierFlags}).
				EvaluateAllTestCases(dir, testCases, c.name, languages.CPP)
			require.Len(t, result.TestResults, len(testCases))
			for i, r := range result.TestResults {
				t.Logf("test %d: status=%d exit=%d peak=%dKB", i+1, r.StatusCode, r.ExitCode, r.PeakMem)
				assert.Equal(t, c.want, r.StatusCode)
			}
			if c.name == "forking over the limit" {
				t.Logf("children holding memory at once: %s", readFile(t, filepath.Join(dir.UserOutputDirPath, "1.out")))
			}
		})
	}

	t.Run("compilation error", func(t *testing.T) {
		dir, testCases := preparePackage(t, `int main() { return missing; }`, []int64{64 * 1024}, "")

		require.NoError(t, exec.NewExecutor(dockerClient, 60).ExecuteCommand(exec.CommandConfig{
			MessageID:         filepath.Base(dir.PackageDirPath),
			DirConfig:         dir,
			LanguageType:      languages.CPP,
			LanguageVersion:   "17",
			TestCases:         testCases,
			SourceFilePath:    dir.UserSolutionPath,
			RequiresCompiling: true,
		}))

		assert.Contains(t, readFile(t, dir.CompileErrFilePath), "missing")
		assert.NoFileExists(t, filepath.Join(dir.UserExecResultDirPath, "1."+constants.ExecutionResultFileExt))
	})
}

// buildCppImage builds the runtime image from this checkout: a changed script has no published image yet.
func buildCppImage(t *testing.T) {
	t.Helper()
	image, err := languages.CPP.GetDockerImage("17")
	require.NoError(t, err)

	buildDir := t.TempDir()
	for name, src := range map[string]string{
		"Dockerfile":   "cpp/Dockerfile",
		"run_tests.sh": "../../runtimescript/run_tests.sh",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(buildDir, name), []byte(readFile(t, src)), 0o644))
	}
	out, err := osexec.CommandContext(t.Context(), "docker", "build", "-t", image, buildDir).CombinedOutput()
	require.NoError(t, err, string(out))
}

// preparePackage lays out a solution the way the packager does. It must live under /tmp, as in the container.
func preparePackage(t *testing.T, source string, memoryKB []int64, expected string) (
	*packager.TaskDirConfig, []messages.TestCase,
) {
	t.Helper()
	base, err := os.MkdirTemp(constants.TmpDirPath, "executor-it-") //nolint:usetesting // t.TempDir is not in /tmp
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(base) })

	dir := &packager.TaskDirConfig{
		TmpDirPath:            constants.TmpDirPath,
		PackageDirPath:        base,
		InputDirPath:          filepath.Join(base, constants.InputDirName),
		OutputDirPath:         filepath.Join(base, constants.OutputDirName),
		UserSolutionPath:      filepath.Join(base, "solution.cpp"),
		UserExecFilePath:      filepath.Join(base, "solution"),
		UserOutputDirPath:     filepath.Join(base, constants.UserOutputDirName),
		UserErrorDirPath:      filepath.Join(base, constants.UserErrorDirName),
		UserDiffDirPath:       filepath.Join(base, constants.UserDiffDirName),
		CompileErrFilePath:    filepath.Join(base, constants.CompileErrFileName),
		UserExecResultDirPath: filepath.Join(base, constants.UserExecResultDirName),
	}
	for _, path := range []string{
		dir.InputDirPath, dir.OutputDirPath, dir.UserOutputDirPath,
		dir.UserErrorDirPath, dir.UserDiffDirPath, dir.UserExecResultDirPath,
	} {
		require.NoError(t, os.Mkdir(path, 0o755))
	}
	write := func(path, content string) {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write(dir.UserSolutionPath, source)
	write(dir.CompileErrFilePath, "")

	testCases := make([]messages.TestCase, len(memoryKB))
	for i, limit := range memoryKB {
		name := func(ext string) messages.FileLocation {
			return messages.FileLocation{Path: fmt.Sprintf("%d.%s", i+1, ext)}
		}
		testCases[i] = messages.TestCase{
			Order:          i + 1,
			InputFile:      name("in"),
			ExpectedOutput: name("ans"),
			StdOutResult:   name("out"),
			StdErrResult:   name("err"),
			DiffResult:     name("diff"),
			TimeLimitMs:    3000,
			MemoryLimitKB:  limit,
		}
		write(filepath.Join(dir.InputDirPath, name("in").Path), "2 3\n")
		write(filepath.Join(dir.OutputDirPath, name("ans").Path), expected+"\n")
	}
	return dir, testCases
}

func repeatLine(line string, n int) string {
	return strings.Repeat(line+"\n", n-1) + line
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

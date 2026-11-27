package executor_test

import (
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	exec "github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/pkg/constants"
	pkgerrors "github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/pkg/languages"
	"github.com/mini-maxit/worker/pkg/messages"
	mocks "github.com/mini-maxit/worker/tests/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// containerMemory runs ExecuteCommand up to CreateContainer and returns the memory limit it requested.
func containerMemory(t *testing.T, lang languages.LanguageType, version string, compiled bool, memKB int64) int64 {
	ctrl := gomock.NewController(t)
	mockDocker := mocks.NewMockDockerClient(ctrl)
	var got int64
	mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil)
	mockDocker.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_, _ any, hostCfg *container.HostConfig, _ string) (string, error) {
			got = hostCfg.Memory
			return "", errors.New("stop after create")
		})

	tc := makeTestCases()[0]
	tc.MemoryLimitKB = memKB
	_ = exec.NewExecutor(mockDocker).ExecuteCommand(exec.CommandConfig{
		MessageID:         "mem",
		DirConfig:         makeDirConfig(),
		LanguageType:      lang,
		LanguageVersion:   version,
		TestCases:         []messages.TestCase{tc},
		RequiresCompiling: compiled,
	})
	return got
}

func TestContainerMemory(t *testing.T) {
	// interpreted: test limit + 64 MB headroom, unchanged
	assert.Equal(t, int64(128*1024*1024), containerMemory(t, languages.PYTHON, "3.12", false, 64*1024))
	// compiled: raised to the compile floor so the compiler isn't OOM-killed
	assert.Equal(t, constants.MinCompileContainerMemoryKB*1024, containerMemory(t, languages.CPP, "17", true, 64*1024))
	// compiled with a limit above the floor: test limit + 20% headroom wins
	assert.Equal(t, int64(1258291*1024), containerMemory(t, languages.CPP, "17", true, 1024*1024))
}

// TestExecuteCommand_Compiled checks the two container runs of a compiled solution.
func TestExecuteCommand_Compiled(t *testing.T) {
	updateErr := errors.New("update failed")
	cases := []struct {
		name      string
		compile   int64 // exit code of the compiling run
		updateErr error
		tests     int64 // exit code of the testing run, if there is one
		runsTests bool
		copies    bool
		want      error
	}{
		{name: "compiles then runs tests", compile: constants.ExitCodeCompiled, runsTests: true, copies: true},
		{name: "compilation error stops after the first run", compile: 0, copies: true},
		{name: "compile run fails", compile: 137, want: pkgerrors.ErrContainerFailed},
		{name: "tests never run if the limit cannot be lowered", compile: constants.ExitCodeCompiled,
			updateErr: updateErr, want: updateErr},
		{name: "test run fails", compile: constants.ExitCodeCompiled, tests: 137, runsTests: true,
			want: pkgerrors.ErrContainerFailed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mockDocker := mocks.NewMockDockerClient(gomock.NewController(t))
			start := func() *gomock.Call { return mockDocker.EXPECT().StartContainer(gomock.Any(), "cid").Return(nil) }
			wait := func(code int64) *gomock.Call {
				return mockDocker.EXPECT().WaitContainer(gomock.Any(), "cid", gomock.Any()).Return(code, nil)
			}

			calls := []any{
				mockDocker.EXPECT().EnsureImage(gomock.Any(), gomock.Any()).Return(nil),
				mockDocker.EXPECT().CreateContainer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("cid", nil),
				mockDocker.EXPECT().CopyToContainerFiltered(
					gomock.Any(), "cid", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
				start(),
				wait(c.compile),
			}
			if c.compile == constants.ExitCodeCompiled {
				// 64 MB test limit + 64 MB headroom, without the compile floor
				calls = append(calls, mockDocker.EXPECT().
					SetContainerMemory(gomock.Any(), "cid", int64(128*1024*1024)).Return(c.updateErr))
			}
			if c.runsTests {
				calls = append(calls, start(), wait(c.tests))
			}
			if c.copies {
				calls = append(calls, mockDocker.EXPECT().CopyFromContainerFiltered(gomock.Any(), "cid", gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil))
			}
			calls = append(calls, mockDocker.EXPECT().ContainerRemove(gomock.Any(), "cid"))
			gomock.InOrder(calls...)

			tc := makeTestCases()[0]
			tc.MemoryLimitKB = 64 * 1024
			err := exec.NewExecutor(mockDocker).ExecuteCommand(exec.CommandConfig{
				MessageID:         "compiled",
				DirConfig:         makeDirConfig(),
				LanguageType:      languages.CPP,
				LanguageVersion:   "17",
				TestCases:         []messages.TestCase{tc},
				RequiresCompiling: true,
			})
			assert.ErrorIs(t, err, c.want)
		})
	}
}

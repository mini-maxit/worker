package executor_test

import (
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	exec "github.com/mini-maxit/worker/internal/stages/executor"
	"github.com/mini-maxit/worker/pkg/constants"
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

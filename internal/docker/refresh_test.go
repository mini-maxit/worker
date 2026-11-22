package docker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/docker"
	mocks "github.com/mini-maxit/worker/tests/mocks"
	"go.uber.org/mock/gomock"
)

func TestRefreshImagesPeriodically(t *testing.T) {
	ctrl := gomock.NewController(t)
	d := mocks.NewMockDockerClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// a failing image must not stop the others from refreshing
	d.EXPECT().RefreshImage(gomock.Any(), "a").Return(errors.New("pull failed")).MinTimes(1)
	d.EXPECT().RefreshImage(gomock.Any(), "b").DoAndReturn(func(context.Context, string) error {
		cancel()
		return nil
	}).MinTimes(1)

	returned := make(chan struct{})
	go func() {
		docker.RefreshImagesPeriodically(ctx, d, []string{"a", "b"}, 10*time.Millisecond)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh loop did not run and stop")
	}
}

package docker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mini-maxit/worker/internal/docker"
	mocks "github.com/mini-maxit/worker/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRefreshImagesPeriodically(t *testing.T) {
	ctrl := gomock.NewController(t)
	d := mocks.NewMockDockerClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	d.EXPECT().RefreshImages(gomock.Any(), []string{"a"}).Do(func(context.Context, []string) {
		if calls++; calls == 2 {
			cancel()
		}
	}).MinTimes(2)

	returned := make(chan struct{})
	go func() {
		docker.RefreshImagesPeriodically(ctx, d, []string{"a"}, 10*time.Millisecond)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh loop did not repeat and stop")
	}
}

// fakeDaemon serves the parts of the Docker API used to pull and prune images.
// Pulling "broken" fails inside a 200 response, as the daemon does; pulling "stalled" never finishes.
type fakeDaemon struct {
	mu           sync.Mutex
	pulled       []string
	pruneFilters []string
}

func (f *fakeDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/images/create"):
		name := r.URL.Query().Get("fromImage")
		f.mu.Lock()
		f.pulled = append(f.pulled, name)
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"Pulling"}` + "\n"))
		switch {
		case strings.HasSuffix(name, "broken"):
			_, _ = w.Write([]byte(`{"errorDetail":{"message":"no space left on device"}}` + "\n"))
		case strings.HasSuffix(name, "stalled"):
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		}
	case strings.HasSuffix(r.URL.Path, "/images/prune"):
		f.mu.Lock()
		f.pruneFilters = append(f.pruneFilters, r.URL.Query().Get("filters"))
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}
}

func newFakeDockerClient(t *testing.T) (docker.DockerClient, *fakeDaemon) {
	daemon := &fakeDaemon{}
	server := httptest.NewServer(daemon)
	t.Cleanup(server.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+server.Listener.Addr().String())
	t.Setenv("DOCKER_API_VERSION", "1.47")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")

	client, err := docker.NewDockerClient()
	require.NoError(t, err)
	return client, daemon
}

func TestRefreshImages(t *testing.T) {
	client, daemon := newFakeDockerClient(t)

	// a failing image must not stop the others, or the cleanup
	client.RefreshImages(context.Background(), []string{"broken", "good"})

	assert.Equal(t, []string{"docker.io/library/broken", "docker.io/library/good"}, daemon.pulled)
	require.Len(t, daemon.pruneFilters, 1)
	// only untagged images that carry our label may be removed
	assert.JSONEq(t,
		`{"dangling":{"true":true},"label":{"org.mini-maxit.runtime=true":true}}`, daemon.pruneFilters[0])
}

func TestEnsureImage_ReportsErrorFromPullStream(t *testing.T) {
	client, _ := newFakeDockerClient(t)

	err := client.EnsureImage(context.Background(), "broken")

	assert.ErrorContains(t, err, "no space left on device")
}

func TestEnsureImage_StalledPullEndsWithContext(t *testing.T) {
	client, _ := newFakeDockerClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := client.EnsureImage(ctx, "stalled")

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

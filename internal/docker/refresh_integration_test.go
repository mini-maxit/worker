//go:build integration

package docker_test

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/mini-maxit/worker/internal/docker"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refreshTestImage is a few kilobytes, so pulling it from scratch is cheap.
const refreshTestImage = "hello-world:latest"

func TestRefreshImages_PullsMissingAndKeepsUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	dc, cli, ctx := newRefreshTestClients(t)
	removeImageIfExists(t, refreshTestImage)

	dc.RefreshImages(ctx, []string{refreshTestImage})
	pulled, err := cli.ImageInspect(ctx, refreshTestImage)
	require.NoError(t, err, "a missing image should be pulled")

	// a tag that doesn't exist must not stop the rest, or change an image that is up to date
	dc.RefreshImages(ctx, []string{"hello-world:maxit-no-such-tag", refreshTestImage})
	unchanged, err := cli.ImageInspect(ctx, refreshTestImage)
	require.NoError(t, err)
	assert.Equal(t, pulled.ID, unchanged.ID)

	assert.Error(t, dc.EnsureImage(ctx, "hello-world:maxit-no-such-tag"))
}

func TestRefreshImages_PrunesOnlyUnusedRuntimeImages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	dc, cli, ctx := newRefreshTestClients(t)
	require.NoError(t, dc.EnsureImage(ctx, refreshTestImage))

	base, err := cli.ContainerCreate(ctx, &container.Config{Image: refreshTestImage}, nil, nil, nil, "")
	require.NoError(t, err)
	t.Cleanup(func() { cleanupContainer(t, base.ID) })

	// commit builds a distinct image under tag, leaving whatever held the tag before untagged
	commit := func(tag, version string, runtimeImage bool) string {
		changes := []string{"ENV MAXIT_TEST=" + version}
		if runtimeImage {
			changes = append(changes, "LABEL "+constants.RuntimeImageLabel)
		}
		res, err := cli.ContainerCommit(ctx, base.ID, container.CommitOptions{Reference: tag, Changes: changes})
		require.NoError(t, err)
		return res.ID
	}
	tags := []string{"maxit-refresh-test:unused", "maxit-refresh-test:used", "maxit-refresh-test:other"}
	t.Cleanup(func() {
		for _, tag := range tags {
			removeImageIfExists(t, tag)
		}
	})

	unused := commit(tags[0], "unused-old", true)
	used := commit(tags[1], "used-old", true)
	unlabelled := commit(tags[2], "other-old", false)
	holder, err := cli.ContainerCreate(ctx, &container.Config{Image: used}, nil, nil, nil, "")
	require.NoError(t, err)
	// cleanups run last-in first-out: the containers go before the images they use
	t.Cleanup(func() {
		cleanupContainer(t, holder.ID)
		for _, id := range []string{used, unlabelled} {
			removeImageByID(t, cli, id)
		}
	})
	current := commit(tags[0], "unused-new", true)
	commit(tags[1], "used-new", true)
	commit(tags[2], "other-new", false)

	dc.RefreshImages(ctx, nil)

	_, err = cli.ImageInspect(ctx, unused)
	require.Error(t, err, "a replaced runtime image should be removed")
	for name, id := range map[string]string{"current": current, "in use": used, "unlabelled": unlabelled} {
		_, err = cli.ImageInspect(ctx, id)
		assert.NoError(t, err, "the %s image should be kept", name)
	}
}

func newRefreshTestClients(t *testing.T) (docker.DockerClient, *client.Client, context.Context) {
	dc, err := docker.NewDockerClient()
	require.NoError(t, err)
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return dc, cli, ctx
}

func removeImageByID(t *testing.T, cli *client.Client, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := cli.ImageRemove(ctx, id, image.RemoveOptions{}); err != nil {
		t.Logf("failed to remove test image %s: %v", id, err)
	}
}

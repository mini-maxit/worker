package docker

import (
	"context"
	"encoding/json"
	e "errors"
	"fmt"
	"io"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	image "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"go.uber.org/zap"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/utils"
)

type DockerClient interface {
	EnsureImage(ctx context.Context, imageName string) error
	RefreshImages(ctx context.Context, imageNames []string)
	CreateContainer(
		ctx context.Context,
		containerCfg *container.Config,
		hostCfg *container.HostConfig,
		name string,
	) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	SetContainerMemory(ctx context.Context, containerID string, memoryBytes int64) error
	WaitContainer(ctx context.Context, containerID string, timeout time.Duration) (int64, error)
	ContainerKill(ctx context.Context, containerID, signal string)
	CopyToContainerFiltered(ctx context.Context, containerID, srcPath, dstPath string, excludeTop []string) error
	CopyFromContainerFiltered(
		ctx context.Context,
		containerID, srcPath, dstPath string,
		allowedDirs []string,
		alwaysCopyFiles []string,
		maxFileSize int64,
		maxFilesInDir int,
	) error
	ContainerRemove(ctx context.Context, containerID string)
}

type dockerClient struct {
	cli    *client.Client
	logger *zap.SugaredLogger
}

func NewDockerClient() (DockerClient, error) {
	logger := logger.NewNamedLogger("docker-client")
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &dockerClient{cli: cli, logger: logger}, nil
}

func (d *dockerClient) EnsureImage(ctx context.Context, imageName string) error {
	_, err := d.cli.ImageInspect(ctx, imageName)
	if err == nil {
		return nil
	}
	if !cerrdefs.IsNotFound(err) {
		d.logger.Errorf("Failed to inspect Docker image %s: %s", imageName, err)
		return err
	}

	return d.pull(ctx, imageName)
}

func (d *dockerClient) pull(ctx context.Context, imageName string) error {
	reader, err := d.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		d.logger.Errorf("Failed to pull Docker image %s: %s", imageName, err)
		return err
	}
	defer reader.Close()

	if err := readPullStream(reader); err != nil {
		d.logger.Errorf("Failed to pull Docker image %s: %s", imageName, err)
		return err
	}
	return nil
}

// readPullStream waits for a pull to finish. The daemon reports failures such as a full disk
// inside the response stream, after it has already answered with success.
func readPullStream(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	for {
		var message struct {
			ErrorDetail *struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		err := decoder.Decode(&message)
		switch {
		case e.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		case message.ErrorDetail != nil:
			return fmt.Errorf("%w: %s", errors.ErrImagePull, message.ErrorDetail.Message)
		}
	}
}

// RefreshImages re-pulls each tag so rebuilt images reach a long-running worker, then removes the runtime
// images this left untagged. Failures are logged and retried on the next call.
// shortcut: images under an older script tag stay tagged, so one set per script change remains on the host.
func (d *dockerClient) RefreshImages(ctx context.Context, imageNames []string) {
	for _, imageName := range imageNames {
		pullCtx, cancel := context.WithTimeout(ctx, constants.RuntimeImagePullTimeoutMinutes*time.Minute)
		_ = d.pull(pullCtx, imageName) // pull logs its own failures
		cancel()
	}

	pruneCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// Prune skips images that a container still uses, so those are retried next time.
	report, err := d.cli.ImagesPrune(pruneCtx, filters.NewArgs(
		filters.Arg("dangling", "true"),
		filters.Arg("label", constants.RuntimeImageLabel),
	))
	if err != nil {
		d.logger.Warnf("Could not remove old runtime images: %s", err)
		return
	}
	if len(report.ImagesDeleted) > 0 {
		d.logger.Infof("Removed old runtime images, reclaimed %d bytes", report.SpaceReclaimed)
	}
}

func (d *dockerClient) CreateContainer(
	ctx context.Context,
	containerCfg *container.Config,
	hostCfg *container.HostConfig,
	name string,
) (string, error) {
	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
	if err != nil {
		d.logger.Errorf("Failed to create Docker container %s: %s", name, err)
		return "", err
	}
	return resp.ID, nil
}

func (d *dockerClient) StartContainer(ctx context.Context, containerID string) error {
	err := d.cli.ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil {
		d.logger.Errorf("Failed to start Docker container %s: %s", containerID, err)
	}
	return err
}

// SetContainerMemory changes the memory limit (no swap on top). A stopped container gets it at its next start.
func (d *dockerClient) SetContainerMemory(ctx context.Context, containerID string, memoryBytes int64) error {
	_, err := d.cli.ContainerUpdate(ctx, containerID, container.UpdateConfig{
		Resources: container.Resources{Memory: memoryBytes, MemorySwap: memoryBytes},
	})
	if err != nil {
		d.logger.Errorf("Failed to set memory limit of Docker container %s: %s", containerID, err)
	}
	return err
}

func (d *dockerClient) WaitContainer(ctx context.Context, containerID string, timeout time.Duration) (int64, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	statusCh, errCh := d.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		d.logger.Errorf("Error while waiting for container %s: %s", containerID, err)
		return -1, err
	case status := <-statusCh:
		d.logger.Infof("Container %s exited with status code %d", containerID, status.StatusCode)
		return status.StatusCode, nil
	case <-ctx.Done():
		d.logger.Errorf("Context done while waiting for container %s: %s", containerID, ctx.Err())
		return -1, ctx.Err()
	case <-timer.C:
		d.logger.Errorf("Timeout while waiting for container %s", containerID)
		return -1, errors.ErrContainerTimeout
	}
}

func (d *dockerClient) ContainerKill(ctx context.Context, containerID, signal string) {
	err := d.cli.ContainerKill(ctx, containerID, signal)
	if err != nil {
		d.logger.Errorf("Failed to kill container %s with signal %s: %s", containerID, signal, err)
	}
}

func (d *dockerClient) CopyToContainerFiltered(
	ctx context.Context,
	containerID, srcPath, dstPath string,
	excludeTop []string,
) error {
	// Create a tar archive from the source directory, preserving the base directory name.
	tar, err := utils.CreateTarArchiveWithBaseFiltered(srcPath, excludeTop)
	if err != nil {
		d.logger.Errorf("Failed to create tar archive from %s: %s", srcPath, err)
		return err
	}
	defer tar.Close()

	// Copy to container
	err = d.cli.CopyToContainer(ctx, containerID, dstPath, tar, container.CopyToContainerOptions{})
	if err != nil {
		d.logger.Errorf("Failed to copy to container %s: %s", containerID, err)
	}
	return err
}

func (d *dockerClient) CopyFromContainerFiltered(
	ctx context.Context,
	containerID, srcPath, dstPath string,
	allowedDirs []string,
	alwaysCopyFiles []string,
	maxFileSize int64,
	maxFilesInDir int,
) error {
	// Get tar archive from container
	reader, _, err := d.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		d.logger.Errorf("Failed to copy from container %s: %s", containerID, err)
		return err
	}
	defer reader.Close()

	// Extract tar archive with filtering to destination
	err = utils.ExtractTarArchiveFiltered(reader, dstPath, allowedDirs, alwaysCopyFiles, maxFileSize, maxFilesInDir)
	if err != nil {
		d.logger.Errorf("Failed to extract filtered tar archive to %s: %s", dstPath, err)
	}
	return err
}

func (d *dockerClient) ContainerRemove(ctx context.Context, containerID string) {
	err := d.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if err != nil {
		d.logger.Errorf("Failed to remove container %s: %s", containerID, err)
	}
}

package docker

import (
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	image "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"go.uber.org/zap"

	"github.com/mini-maxit/worker/internal/logger"
	"github.com/mini-maxit/worker/pkg/errors"
	"github.com/mini-maxit/worker/utils"
)

type DockerClient interface {
	EnsureImage(ctx context.Context, imageName string) error
	CreateContainer(
		ctx context.Context,
		containerCfg *container.Config,
		hostCfg *container.HostConfig,
		name string,
	) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	WaitContainer(ctx context.Context, containerID string, timeout time.Duration) (int64, error)
	ContainerKill(ctx context.Context, containerID, signal string)
	CopyToContainerFiltered(ctx context.Context, containerID, srcPath, dstPath string, excludeTop []string) error
	CopyFromContainer(ctx context.Context, containerID, srcPath, dstPath string) error
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
	if !client.IsErrNotFound(err) {
		d.logger.Errorf("Failed to inspect Docker image %s: %s", imageName, err)
		return err
	}

	reader, err := d.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		d.logger.Errorf("Failed to pull Docker image %s: %s", imageName, err)
		return err
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)

	if err != nil {
		d.logger.Errorf("Failed to read Docker image pull response for image %s: %s", imageName, err)
	}

	return err
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

func (d *dockerClient) CopyFromContainer(ctx context.Context, containerID, srcPath, dstPath string) error {
	// Get tar archive from container
	reader, _, err := d.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		d.logger.Errorf("Failed to copy from container %s: %s", containerID, err)
		return err
	}
	defer reader.Close()

	// Extract tar archive to destination
	err = utils.ExtractTarArchive(reader, dstPath)
	if err != nil {
		d.logger.Errorf("Failed to extract tar archive to %s: %s", dstPath, err)
	}
	return err
}

func (d *dockerClient) ContainerRemove(ctx context.Context, containerID string) {
	err := d.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	if err != nil {
		d.logger.Errorf("Failed to remove container %s: %s", containerID, err)
	}
}

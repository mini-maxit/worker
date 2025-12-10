package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	image "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/mini-maxit/worker/utils"
)

type DockerClient interface {
	EnsureImage(ctx context.Context, imageName string) error
	CreateAndStartContainer(
		ctx context.Context,
		containerCfg *container.Config,
		hostCfg *container.HostConfig,
		name string,
	) (string, error)
	WaitContainer(ctx context.Context, containerID string) (int64, error)
	ContainerWait(
		ctx context.Context,
		containerID string,
		condition container.WaitCondition,
	) (<-chan container.WaitResponse, <-chan error)
	ContainerKill(ctx context.Context, containerID, signal string) error
	CopyToContainer(ctx context.Context, containerID, srcPath, dstPath string) error
	CopyFromContainer(ctx context.Context, containerID, srcPath, dstPath string) error
	ContainerRemove(ctx context.Context, containerID string) error
}

type dockerClient struct {
	cli *client.Client
}

func NewDockerClient() (DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	return &dockerClient{cli: cli}, nil
}

func (d *dockerClient) EnsureImage(ctx context.Context, imageName string) error {
	_, err := d.cli.ImageInspect(ctx, imageName)
	if err == nil {
		return nil
	}
	if !client.IsErrNotFound(err) {
		return err
	}

	reader, err := d.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(io.Discard, reader)
	return err
}

func (d *dockerClient) CreateAndStartContainer(
	ctx context.Context,
	containerCfg *container.Config,
	hostCfg *container.HostConfig,
	name string,
) (string, error) {
	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", err
	}
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (d *dockerClient) WaitContainer(ctx context.Context, containerID string) (int64, error) {
	statusCh, errCh := d.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return -1, err
	case status := <-statusCh:
		return status.StatusCode, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (d *dockerClient) ContainerWait(
	ctx context.Context,
	containerID string,
	condition container.WaitCondition,
) (<-chan container.WaitResponse, <-chan error) {
	return d.cli.ContainerWait(ctx, containerID, condition)
}

func (d *dockerClient) ContainerKill(ctx context.Context, containerID, signal string) error {
	return d.cli.ContainerKill(ctx, containerID, signal)
}

func (d *dockerClient) CopyToContainer(ctx context.Context, containerID, srcPath, dstPath string) error {
	// Create a tar archive from the source directory, preserving the base directory name
	tar, err := utils.CreateTarArchiveWithBase(srcPath)
	if err != nil {
		return err
	}
	defer tar.Close()

	// Copy to container
	return d.cli.CopyToContainer(ctx, containerID, dstPath, tar, container.CopyToContainerOptions{})
}

func (d *dockerClient) CopyFromContainer(ctx context.Context, containerID, srcPath, dstPath string) error {
	// Get tar archive from container
	reader, _, err := d.cli.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Extract tar archive to destination
	return utils.ExtractTarArchive(reader, dstPath)
}

func (d *dockerClient) ContainerRemove(ctx context.Context, containerID string) error {
	return d.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

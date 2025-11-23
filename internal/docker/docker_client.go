package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	image "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/mini-maxit/worker/pkg/constants"
	"github.com/mini-maxit/worker/pkg/errors"
)

type DockerClient interface {
	DataVolumeName() string
	CheckDataVolume(volumeName string) error
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
}

type dockerClient struct {
	cli        *client.Client
	volumeName string
}

func NewDockerClient(volumeName string) (DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	dc := &dockerClient{cli: cli, volumeName: volumeName}
	if volumeName != "" {
		if err := dc.CheckDataVolume(volumeName); err != nil {
			return nil, err
		}
	}

	return dc, nil
}

func (d *dockerClient) DataVolumeName() string { return d.volumeName }

func (d *dockerClient) CheckDataVolume(volumeName string) error {
	ctx := context.Background()
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		for _, m := range c.Mounts {
			if m.Name == volumeName && m.Destination == constants.TmpDirPath {
				return nil
			}
		}
	}

	return errors.ErrVolumeNotMounted
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

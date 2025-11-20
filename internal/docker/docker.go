package docker

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type DockerClient interface {
	CreateClient() (*client.Client, error)
	CreateDataVolume(volumeName string, cli *client.Client) error
	PullImage(imageName string, cli *client.Client, ctx context.Context) error
	CreateAndStartContainer(config *container.Config, cli *client.Client) (string, error)
}

type dockerClient struct {
	cli *client.Client
}



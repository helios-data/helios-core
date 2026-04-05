package docker

import (
	"context"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// Get the context of the Docker client.
func (c *DockerClient) GetContext() context.Context {
	return c.ctx
}

// Get the docker client of the Docker client.
func (c *DockerClient) GetDockerClient() *client.Client {
	return c.cli
}

// Get the network configuration of the Docker client.
func (c *DockerClient) GetNetwork() network.CreateResponse {
	return c.net
}

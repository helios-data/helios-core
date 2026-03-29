package dockerhandler

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"helios/generated/config"
	"helios/internal/commhandler"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type DockerClient struct {
	cli *client.Client
	ctx context.Context
	net network.CreateResponse
}

type ComponentObject struct {
	Mu           sync.RWMutex
	ContainerID  string // Docker ID
	ComponentID  string
	Group        string // Tree group
	Path         string // Path to component code, is this necessary?
	Tag          string // TODO: Do we want to keep this?
	Volumes      []*config.Volume // List of volume mappings for the component
	Ports        []*config.Port // List of port mappings for the component
	CommHandler  *commhandler.CommClient
	SkipSpawn		 bool // Flag to indicate whether to skip spawning this component
}

// Initialize the Docker client.
func (c *DockerClient) Initialize() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	c.cli = cli
	c.ctx = ctx
}

// Close the Docker client.
func (c *DockerClient) Close() {
	c.cli.Close()
}

// Get the ID of an existing container given it's name.
// Returns the ID if found and "" if it does not exist.
func (c *DockerClient) GetContainerID(containerName string) (containerID string) {
	list := c.GetContainers()
	var contID string = ""

	for _, cont := range list {
		if cont.Names[0] == "/"+containerName {
			contID = cont.ID
			break
		}
	}

	return contID
}

// Get a list of all containers.
func (c *DockerClient) GetContainers() (summary []container.Summary) {
	list, err := c.cli.ContainerList(c.ctx, container.ListOptions{All: true})
	if err != nil {
		panic(err)
	}
	return list
}

// Create a container using information from the image struct and runtime_hash.
// It should be checked if a container already exists with the same name and hash before calling this function.
func (c *DockerClient) createContainer(name string, component *ComponentObject, hash string) (response container.CreateResponse, error error) {
	var deviceMappings []container.DeviceMapping

	// Port bindings
	for _, port := range component.Ports {
		var mode string = "rwm"
		// TODO: Implement different modes for port mappings if necessary
		// if path.Mode != "" {
		// 	mode = path.Mode
		// }

		deviceMappings = append(deviceMappings, container.DeviceMapping{
			PathOnHost:        port.Source,
			PathInContainer:   port.Target,
			CgroupPermissions: mode,
		})
	}

	// Volume bindings
	var volumeBinds []string
	for _, volume := range component.Volumes {
		var mode string = "rw"
		volumeBinds = append(volumeBinds, fmt.Sprintf("%s:%s:%s", volume.Source, volume.Target, mode))
	}

	// Create container
	resp, err := c.cli.ContainerCreate(c.ctx,
		&container.Config{
			Image: strings.ToLower(name),
			Labels: map[string]string{
				"runtime_hash": hash,
			},
		},
		&container.HostConfig{
			Binds: volumeBinds,
			Resources: container.Resources{
				Devices: deviceMappings,
			},
		},
		nil, nil, name)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// Start a container given its name, tag, and runtime_hash.
// If the container already exists, it will be restarted or removed and recreated if the hash does not match.
// Returns the container ID of the started container.
// Created container will be added to the docker network and started.
func (c *DockerClient) StartContainer(name string, component *ComponentObject, hash string) (ID string) {
	list := c.GetContainers()
	var cont container.Summary = container.Summary{}

	// Find container by name to see if it exists
	for _, x := range list {
		if x.Names[0] == "/"+name {
			cont = x
			break
		}
	}

	// If the runtime hash does not match
	if (cont.ID != "") && (cont.Labels["runtime_hash"] != hash) {
		// Remove the container
		if err := c.cli.ContainerRemove(c.ctx, cont.ID, container.RemoveOptions{Force: true}); err != nil {
			fmt.Println("Error removing outdated container:", err)
		}

		// Reset cont to indicate it does not exist
		cont = container.Summary{}

		// If the runtime hash matches, check if is exited or running
	} else if cont.ID != "" {
		if cont.State == "running" {
			// Do nothing
		} else if cont.State == "exited" {
			// Restart it
			c.startDockerContainer(cont.ID)
		}
	}

	// If container does not exist, create it
	if cont.ID == "" {
		contResp, contErr := c.createContainer(name, component, hash)
		if contErr != nil {
			// TODO: Handle error properly
			fmt.Println("Error creating container:", contErr)
		}
		cont.ID = contResp.ID
	}

	// Add to network and start container
	go c.AddContainerToNetwork(cont.ID)
	go c.startDockerContainer(cont.ID)
	return cont.ID
}

// Start a docker container by ID.
func (c *DockerClient) startDockerContainer(ID string) {

	// Start container
	if err := c.cli.ContainerStart(c.ctx, ID, container.StartOptions{}); err != nil {
		panic(err)
	}

	// Wait until it finishes
	waitStatusCh, waitErrCh := c.cli.ContainerWait(c.ctx, ID, container.WaitConditionNotRunning)
	select {
	case err := <-waitErrCh:
		if err != nil {
			panic(err)
		}
	case <-waitStatusCh:
	}

	// Get logs
	//TODO: Move this to a seperate logging driver later
	out, err := c.cli.ContainerLogs(c.ctx, ID, container.LogsOptions{ShowStdout: true})
	if err != nil {
		panic(err)
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, out)
}

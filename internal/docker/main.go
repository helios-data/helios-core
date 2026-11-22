package docker

import (
	"context"
	"fmt"
	"path/filepath"
	"io"

	componenttree "helios/internal/component_tree"
	"helios/internal/logger"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type DockerClient struct {
	runtimeHash   string
	cli           *client.Client
	ctx           context.Context
	net           network.CreateResponse
	deviceMonitor *DeviceMonitor
}

/*
 * ========================================================
 * = Public methods for managing Docker runtime
 * ========================================================
 */

func NewDockerClient(ctx context.Context, hash string) *DockerClient {
	return &DockerClient{
		ctx:         ctx,
		runtimeHash: hash,
	}
}

// Initialize the Docker client.
func (c *DockerClient) Initialize() error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	c.cli = cli
	return nil
}

// Start configured components in Docker based on the component tree in the core.
func (c *DockerClient) StartContainers(t *componenttree.ComponentTree) error {
	if err := t.EachComponent(c.startContainer); err != nil {
		return err
	}
	c.deviceMonitor = newDeviceMonitor(c.ctx, c, t)
	if err := c.deviceMonitor.start(); err != nil {
		logger.Warnw("Device monitor failed to start", "error", err)
	}
	return nil
}

func (c *DockerClient) AddContainerToNetworkByName(containerName string) error {
	id := c.getContainerID(containerName)
	if id == "" {
		return fmt.Errorf("Container with name %s not found", containerName)
	}
	
	if err := c.AddContainerToNetwork(id); err != nil {
		return err
	}
	return nil
}

// Close the Docker client.
func (c *DockerClient) Close() error {
	if c.deviceMonitor != nil {
		c.deviceMonitor.stop()
	}
	return c.cli.Close()
}


/*
 * ========================================================
 * = Internal structures for managing component state and connections
 * ========================================================
 */


// Get the ID of an existing container given it's name.
// Returns the ID if found and "" if it does not exist.
func (c *DockerClient) getContainerID(containerName string) (containerID string) {
	list := c.getContainers()
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
func (c *DockerClient) getContainers() (summary []container.Summary) {
	list, err := c.cli.ContainerList(c.ctx, container.ListOptions{All: true})
	if err != nil {
		panic(err)
	}
	return list
}

// Start a container given a pointer to the component struct.
// If the container already exists, it will be restarted or removed and recreated if the hash does not match.
// Created container will be added to the docker network and started.
func (c *DockerClient) startContainer(address string, name string, comp *componenttree.Component) error {
	spec := comp.GetDockerSpec()
	if spec == nil {
		return fmt.Errorf("No DockerSpec found for component at address %s", address)
	}

	// The container name comes from the spec; fall back to the tree node name.
	containerName := spec.GetContainerName()
	if containerName == "" {
		containerName = name
	}

	list := c.getContainers()
	var cont container.Summary = container.Summary{}

	// Find container by name to see if it exists
	for _, x := range list {
		if x.Names[0] == "/"+containerName {
			cont = x
			break
		}
	}

	// If container exists, handle the various cases
	if (cont.ID != "") {
		// If the runtime hash does not match
		if cont.Labels["runtime_hash"] != c.runtimeHash {
			// Remove the container
			if err := c.cli.ContainerRemove(c.ctx, cont.ID, container.RemoveOptions{Force: true}); err != nil {
				return err
			}

			// Reset cont to indicate it does not exist
			cont = container.Summary{}
		
		// If the runtime hash matches, check if is exited or running
		} else {
			if cont.State == "running" {
				// Do nothing
				comp.SetDockerConnID(cont.ID)
				return nil
			} else if cont.State == "exited" {
				// Restart it
				if err := c.startDockerContainer(cont.ID); err != nil {
					return err
				}
				comp.SetDockerConnID(cont.ID)
				c.applyInitialDeviceNodes(cont.ID, comp.GetDockerSpec().GetDevices())
				return nil
			} else if cont.State == "created" {
				// Start it
				if err := c.startDockerContainer(cont.ID); err != nil {
					return err
				}
				comp.SetDockerConnID(cont.ID)
				c.applyInitialDeviceNodes(cont.ID, comp.GetDockerSpec().GetDevices())
				return nil
			}
		}
	}

	// Container does not exist, create it
	contResp, contErr := c.createContainer(address, containerName, comp)
	if contErr != nil {
		return contErr
	}
	cont.ID = contResp.ID

	comp.SetDockerConnID(cont.ID)

	// Add to network and start container
	if err := c.AddContainerToNetwork(cont.ID); err != nil {
		return err
	}
	if err := c.startDockerContainer(cont.ID); err != nil {
		return err
	}
	c.applyInitialDeviceNodes(cont.ID, comp.GetDockerSpec().GetDevices())
	return nil
}

// Create a container using information from the image struct and runtime_hash.
// It should be checked if a container already exists with the same name and hash before calling this function.
func (c *DockerClient) createContainer(address string, containerName string, comp *componenttree.Component) (container.CreateResponse, error) {
	spec := comp.GetDockerSpec()
	if spec == nil {
		return container.CreateResponse{}, fmt.Errorf("No DockerSpec found for component at address %s", address)
	}

	// Build the image reference from the spec (image[:tag]).
	image := spec.GetImage()
	if image == "" {
		return container.CreateResponse{}, fmt.Errorf("DockerSpec for component at address %s has no image", address)
	}
	if tag := spec.GetTag(); tag != "" {
		image = image + ":" + tag
	}

	// Device configuration (platform-specific: cgroup rules on Linux, bind mappings elsewhere)
	deviceMappings, cgroupRules, deviceBinds := buildDeviceConfig(spec.Devices)

	// Volume bindings (device directory binds are merged in)
	var volumeBinds []string
	for _, volume := range spec.Volumes {
		volumeBinds = append(volumeBinds, fmt.Sprintf("%s:%s:rw", volume.Source, volume.Target))
	}
	volumeBinds = append(volumeBinds, deviceBinds...)

	// Environment variables (KEY=VALUE form expected by Docker)
	var envVars []string
	for key, value := range spec.GetEnv() {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	// Port forwarding
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, port := range spec.Ports {
		portType := port.Type
		if portType == "" {
			portType = "tcp"
		}
		containerPort := nat.Port(fmt.Sprintf("%s/%s", port.Target, portType))
		exposedPorts[containerPort] = struct{}{}
		portBindings[containerPort] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: port.Source},
		}
	}

	// Create container
	resp, err := c.cli.ContainerCreate(c.ctx,
		&container.Config{
			Image:        image,
			Cmd:          comp.GetFlags(),
			Env:          envVars,
			ExposedPorts: exposedPorts,
			Labels: map[string]string{
				"runtime_hash": c.runtimeHash,
			},
		},
		&container.HostConfig{
			Binds:        volumeBinds,
			PortBindings: portBindings,
			Resources: container.Resources{
				Devices:           deviceMappings,
				DeviceCgroupRules: cgroupRules,
			},
		},
		nil, nil, containerName)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// Start a docker container by ID.
func (c *DockerClient) startDockerContainer(ID string) error {
	if err := c.cli.ContainerStart(c.ctx, ID, container.StartOptions{}); err != nil {
		return err
	}
	return nil
}

// execMknod runs `mknod` inside a container via a privileged exec to create a
// device node at target with the given character device major:minor numbers.
func (c *DockerClient) execMknod(containerID string, target string, major uint32, minor uint32) error {
	cmd := fmt.Sprintf("mkdir -p %s && rm -f %s && mknod -m 660 %s c %d %d", filepath.Dir(target), target, target, major, minor)
	execID, err := c.cli.ContainerExecCreate(c.ctx, containerID, container.ExecOptions{
		Cmd:        []string{"/bin/sh", "-c", cmd},
		Privileged: true,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}
	resp, err := c.cli.ContainerExecAttach(c.ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()
	io.Copy(io.Discard, resp.Reader)

	inspect, err := c.cli.ContainerExecInspect(c.ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("exec inspect: %w", err)
	}
	if inspect.ExitCode != 0 {
		return fmt.Errorf("mknod exited with code %d", inspect.ExitCode)
	}
	return nil
}

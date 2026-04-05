package docker

import (
	"fmt"
	"net"
	"strings"

	"github.com/docker/docker/api/types/network"
)

type NewConnection struct {
	Name string
	ID   string
	Conn net.Conn
}

// Start up a Docket network with the provided name.
// If it already exists, it finds and returns the ID of the existing network.
// Network will be saved into the client object.
// Returns the network and any errors
func (c *DockerClient) StartDockerNetwork(networkName string) (resp network.CreateResponse, errResp error) {
	list, netErr := c.cli.NetworkList(c.ctx, network.ListOptions{})
	if netErr != nil {
		panic(netErr)
	}

	for _, net := range list {
		if net.Name == networkName {
			fmt.Println("'", networkName, "' network already exists:", net.ID)
			c.net = network.CreateResponse{ID: net.ID}
			return c.net, nil
		}
	}

	// Create network
	fmt.Println("Creating Docker network '", networkName, "'...")
	result, err := c.cli.NetworkCreate(c.ctx, networkName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return network.CreateResponse{}, err
	}
	fmt.Println("'", networkName, "' network created:", resp.ID)

	c.net = result
	return result, nil
}

// Add a container to the specified Docker Network
func (c *DockerClient) AddContainerToNetwork(containerID string) {
	err := c.cli.NetworkConnect(c.ctx, c.net.ID, containerID, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Container %s connected to network %s\n", containerID, c.net.ID)
}

// Get a container's name and ID from it's local IP address
func (c *DockerClient) GetContainerInfoFromIP(ip string) (name string, id string) {
	list := c.GetContainers()
	x := strings.Split(ip, ":")

	for _, c := range list {
		for _, net := range c.NetworkSettings.Networks {
			if net.IPAddress == x[0] {
				return c.Names[0][1:], c.ID
			}
		}
	}
	return "", ""
}

// Start listening to a new port connection to the specified port.
// New network connections will be send to the out channel.
func (c *DockerClient) StartPortConnection(port string, out chan NewConnection) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
	}
	defer listener.Close()

	fmt.Printf("TCP server is listening on port %s\n", port)

	for {
		// Wait for and accept a new client connection
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v", err)
			continue
		}

		go func(conn net.Conn) {
			name, id := c.GetContainerInfoFromIP(conn.RemoteAddr().String())
			fmt.Printf("New connection from container '%s' (%s)\n", name, id)

			out <- NewConnection{
				Name: name,
				ID:   id,
				Conn: conn,
			}
		}(conn)
	}
}

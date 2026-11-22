package component_tree

import (
	"net"
	"sync"

	"helios/generated/config"
	"helios/generated/transport"
	transportpb "helios/generated/transport"
)

type Component struct {
	mu         sync.RWMutex
	dockerSpec *config.DockerSpec
	flags      []string
	dockerConn *DockerConn
	conn       net.Conn
	events     map[string]*transportpb.Event
}

type DockerConn struct {
	ContainerID string
}

type TransportConn struct {
	Conn net.Conn
}

func NewComponent(dockerSpec *config.DockerSpec, flags []string) *Component {
	return &Component{
		dockerSpec: dockerSpec,
		flags:      flags,
		events:     make(map[string]*transport.Event),
	}
}

func (c *Component) attachConnection(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn = conn
}

func (c *Component) detachConnection() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn = nil
}

func (c *Component) setEvent(eventName string, event *transportpb.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events[eventName] = event
}

func (c *Component) getEvent(eventName string) (*transportpb.Event, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	event, ok := c.events[eventName]
	return event, ok
}

func (c *Component) GetDockerSpec() *config.DockerSpec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dockerSpec
}

func (c *Component) GetFlags() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.flags
}

func (c *Component) SetDockerConnID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// TODO: Check if DockerConn already exists and handle accordingly
	c.dockerConn = &DockerConn{ContainerID: id}
}

func (c *Component) GetDockerConnID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dockerConn == nil {
		return ""
	}
	return c.dockerConn.ContainerID
}

func (c *Component) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	return err
}

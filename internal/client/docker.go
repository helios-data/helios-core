package client

import (
	"fmt"
	"sync"

	"helios/generated/config"
	"helios/internal/commhandler"
	dh "helios/internal/dockerhandler"
)

const (
	SELF_NAME = "Helios"
	NET_NAME  = "HeliosNet"
	PORT      = "5000"
)

type DockerInterface struct {
	dc          *dh.DockerClient
	runtimeHash string
	tree        map[string]*dh.ComponentObject // placeholder for local tree object; includes connection object, cont. id, etc
	treeMu      sync.RWMutex                // protects the map itself
	broadcast   map[string]map[string] chan []byte    // map of component name to broadcast channel for sending messages to all instances of a component
	broadcastMu sync.RWMutex                // protects the broadcast map
}

func initializeDockerClient(hash string) *DockerInterface {
	return &DockerInterface{
		dc:          nil,
		runtimeHash: hash,
		tree:        make(map[string]*dh.ComponentObject),
		broadcast:   make(map[string]map[string]chan []byte),
	}
}

func (x *DockerInterface) Initialize() {
	// Create a new docker client
	x.dc = &dh.DockerClient{}
	x.dc.Initialize()

	// Start the docker network
	_, netErr := x.dc.StartDockerNetwork(NET_NAME)
	if netErr != nil {
		panic(netErr)
	}

	connectionChan := make(chan dh.NewConnection)
	go x.dc.StartPortConnection(PORT, connectionChan)
	go x.handleNewConnection(connectionChan)

	// Get own container's ID
	selfId := x.dc.GetContainerID(SELF_NAME)
	if selfId == "" {
		panic(SELF_NAME + "ID not found.")
	}

	// Add self to the network
	x.dc.AddContainerToNetwork(selfId)
}

func (x *DockerInterface) InitializeComponentTree(path string) {
	tree := extractComponentTree(path)

	// Recursively add children to tree
	x.addTreeNodes(tree.Root, SELF_NAME)

	//! Temp: Print out component tree for debugging
	fmt.Println("Initialized component tree:", x.tree)
	for name, component := range x.tree {
		fmt.Printf("Component Name: %s, Component ID: %s, Group: %s, Path: %s, Tag: %s\nVolumes: %v, Ports: %v", name, component.ComponentID, component.Group, component.Path, component.Tag, component.Volumes, component.Ports)
	}
}

func (x *DockerInterface) StartComponent(name string) {
	x.treeMu.RLock()
	c := x.tree[name]
	x.treeMu.RUnlock()

	if c == nil {
		return
	}

	// Start the container through docker handler and add to docker network
	id := x.dc.StartContainer(name, c, x.runtimeHash)
		
	c.Mu.Lock()
	c.ContainerID = id
	c.Mu.Unlock()
}

func (x *DockerInterface) StartAllComponents() {
	x.treeMu.RLock()

	//Check if the component tree has been inititalized yet before running
	if len(x.tree) == 0 {
		x.treeMu.RUnlock()
		return
	}

	names := make([]string, 0, len(x.tree))
	for name := range x.tree {
		names = append(names, name)
	}
	x.treeMu.RUnlock()

	// TODO: Do we want to wait for all components to have started before returning?
	for _, name := range names {
		go x.StartComponent(name)
	}
}

func (x *DockerInterface) StopComponent(name string) {
	// Stop an individual component
	// docker stop
	// Send a message over to component to get it to stop
}

func (x *DockerInterface) KillComponent(name string) {
	// Kill? an individual component (Stop, remove)
	// docker kill
	// Force kill
}

func (x *DockerInterface) Clean() {
	// Clean component (s)?
}

// Close the docker client
func (x *DockerInterface) Close() {
	x.dc.Close()
}

// Recursively checks all branches and adds children to the tree object
func (x *DockerInterface) addTreeNodes(node *config.BaseComponent, group string) {
	switch v := node.NodeType.(type) {
	case *config.BaseComponent_Branch:
		for _, c := range v.Branch.Children {
			x.addTreeNodes(c, group+"."+node.Name)
		}
	case *config.BaseComponent_Leaf:
		obj := &dh.ComponentObject{
			Group:       group,
			Path:        v.Leaf.Path,
			Tag:         v.Leaf.Tag,
			ComponentID: v.Leaf.Id,
			Volumes:     v.Leaf.Volumes,
			Ports:       v.Leaf.Ports,
		}

		x.treeMu.Lock()
		x.tree[node.Name] = obj
		x.treeMu.Unlock()
	}
}

// Handles new connections made by Docker through an update channel.
// If the component does not existing in the tree by name, a new one is made.
// A new communications handler is spawned with the connection object and saved to the component.
func (x *DockerInterface) handleNewConnection(conn chan dh.NewConnection) {
	for {
		c := <-conn

		// Check if component exists in tree
		x.treeMu.RLock()
		comp, ok := x.tree[c.Name]
		x.treeMu.RUnlock()

		if ok {
			comp.Mu.Lock()

			// Check if an empty communications handler was initialized
			if comp.CommHandler == nil {
				comp.CommHandler = commhandler.NewCommClient(c.Conn)
			} else {
				comp.CommHandler.SetConn(c.Conn)
			}

			comp.Mu.Unlock()
		} else {
			// Need to add the new component safely
			x.treeMu.Lock()
			// re-check in case another goroutine added it
			comp, ok = x.tree[c.Name]
			if !ok {
				x.tree[c.Name] = &dh.ComponentObject{
					CommHandler: commhandler.NewCommClient(c.Conn),
				}
				x.treeMu.Unlock()
			} else {
				x.treeMu.Unlock()
				comp.Mu.Lock()
				comp.CommHandler = commhandler.NewCommClient(c.Conn)
				comp.Mu.Unlock()
			}
		}
	}
}

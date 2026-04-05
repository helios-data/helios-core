package client

import (
	"fmt"
	"sync"

	"helios/generated/config"
	"helios/internal/docker"
	"helios/internal/transport"
)

const (
	SELF_NAME = "Helios"
	NET_NAME  = "HeliosNet"
	PORT      = "5000"
)

type DockerInterface struct {
	dc          *docker.DockerClient
	runtimeHash string
	tree        map[string]*docker.ComponentObject // placeholder for local tree object; includes connection object, cont. id, etc
	treeMu      sync.RWMutex                // protects the map itself
	broadcast   map[string]map[string] chan []byte    // map of component name to broadcast channel for sending messages to all instances of a component
	broadcastMu sync.RWMutex                // protects the broadcast map
}

func initializeDockerClient(hash string) *DockerInterface {
	return &DockerInterface{
		dc:          nil,
		runtimeHash: hash,
		tree:        make(map[string]*docker.ComponentObject),
		broadcast:   make(map[string]map[string]chan []byte),
	}
}

func (di *DockerInterface) Initialize() {
	// Create a new docker client
	di.dc = &docker.DockerClient{}
	di.dc.Initialize()

	// Start the docker network
	_, netErr := di.dc.StartDockerNetwork(NET_NAME)
	if netErr != nil {
		panic(netErr)
	}

	connectionChan := make(chan docker.NewConnection)
	go di.dc.StartPortConnection(PORT, connectionChan)
	go di.handleNewConnection(connectionChan)

	// Get own container's ID
	selfId := di.dc.GetContainerID(SELF_NAME)
	if selfId == "" {
		panic(SELF_NAME + "ID not found.")
	}

	// Add self to the network
	di.dc.AddContainerToNetwork(selfId)
}

func (di *DockerInterface) InitializeComponentTree(path string) {
	tree := extractComponentTree(path)

	// Recursively add children to tree
	di.addTreeNodes(tree.Root, SELF_NAME)

	//! Temp: Print out component tree for debugging
	fmt.Println("Initialized component tree:", di.tree)
	for name, component := range di.tree {
		fmt.Printf("==============\n")
		fmt.Printf("Component Name: %s\n", name)
		fmt.Printf("Component ID: %s\n", component.ComponentID)
		fmt.Printf("Group: %s\n", component.Group)
		fmt.Printf("Path: %s\n", component.Path)
		fmt.Printf("Tag: %s\n", component.Tag)
		fmt.Printf("Volumes: %v\n", component.Volumes)
		fmt.Printf("Ports: %v\n", component.Ports)
		fmt.Printf("==============\n")
	}
}

func (di *DockerInterface) StartComponent(name string) {
	di.treeMu.RLock()
	c := di.tree[name]
	di.treeMu.RUnlock()

	if c == nil {
		return
	}

	if c.SkipSpawn {
		fmt.Printf("Skipping spawn of component %s due to skip_spawn flag.\n", name)
		return
	}

	// Start the container through docker handler and add to docker network
	id := di.dc.StartContainer(name, c, di.runtimeHash)

	c.Mu.Lock()
	c.ContainerID = id
	c.Mu.Unlock()
}

func (di *DockerInterface) StartAllComponents() {
	di.treeMu.RLock()

	//Check if the component tree has been inititalized yet before running
	if len(di.tree) == 0 {
		di.treeMu.RUnlock()
		return
	}

	names := make([]string, 0, len(di.tree))
	for name := range di.tree {
		names = append(names, name)
	}
	di.treeMu.RUnlock()

	// TODO: Do we want to wait for all components to have started before returning?
	for _, name := range names {
		go di.StartComponent(name)
	}
}

func (di *DockerInterface) StopComponent(name string) {
	// Stop an individual component
	// docker stop
	// Send a message over to component to get it to stop
}

func (di *DockerInterface) KillComponent(name string) {
	// Kill? an individual component (Stop, remove)
	// docker kill
	// Force kill
}

func (di *DockerInterface) Clean() {
	// Clean component (s)?
}

// Close the docker client
func (di *DockerInterface) Close() {
	di.dc.Close()
}

// Recursively checks all branches and adds children to the tree object
func (di *DockerInterface) addTreeNodes(node *config.BaseComponent, group string) {
	switch v := node.NodeType.(type) {
	case *config.BaseComponent_Branch:
		for _, c := range v.Branch.Children {
			di.addTreeNodes(c, group+"."+node.Name)
		}
	case *config.BaseComponent_Leaf:
		obj := &docker.ComponentObject{
			Group:       group,
			Path:        v.Leaf.Path,
			Tag:         v.Leaf.Tag,
			ComponentID: v.Leaf.Id,
			Volumes:     v.Leaf.Volumes,
			Ports:       v.Leaf.Ports,
			SkipSpawn:   v.Leaf.SkipSpawn,
		}

		di.treeMu.Lock()
		di.tree[node.Name] = obj
		di.treeMu.Unlock()
	}
}

// Handles new connections made by Docker through an update channel.
// If the component does not existing in the tree by name, a new one is made.
// A new communications handler is spawned with the connection object and saved to the component.
func (di *DockerInterface) handleNewConnection(conn chan docker.NewConnection) {
	for {
		c := <-conn

		// Check if component exists in tree
		di.treeMu.RLock()
		comp, ok := di.tree[c.Name]
		di.treeMu.RUnlock()

		if ok {
			comp.Mu.Lock()

			// Check if an empty communications handler was initialized
			if comp.CommHandler == nil {
				comp.CommHandler = transport.NewCommClient(c.Conn)
			} else {
				comp.CommHandler.SetConn(c.Conn)
			}

			comp.Mu.Unlock()
		} else {
			// Need to add the new component safely
			di.treeMu.Lock()
			// re-check in case another goroutine added it
			comp, ok = di.tree[c.Name]
			if !ok {
				di.tree[c.Name] = &docker.ComponentObject{
					CommHandler: transport.NewCommClient(c.Conn),
				}
				di.treeMu.Unlock()
			} else {
				di.treeMu.Unlock()
				comp.Mu.Lock()
				comp.CommHandler = transport.NewCommClient(c.Conn)
				comp.Mu.Unlock()
			}
		}
	}
}

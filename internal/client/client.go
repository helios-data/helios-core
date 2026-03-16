package client

import (
	"os"

	"helios/generated/config"
	"google.golang.org/protobuf/encoding/protojson"
)

type Client interface {
	Initialize()
	InitializeComponentTree(path string)
	StartComponent(name string)
	StartAllComponents()
	StopComponent(name string)
	KillComponent(name string)
	Clean()
	Close()
}

func Initialize(env string, runtime_hash string) Client {
	var cli Client
	switch env {
	case "local":
		cli = initializeLocalClient(runtime_hash)
	case "docker":
		cli = initializeDockerClient(runtime_hash)
		cli.Initialize()
	}
	return cli
}

func extractComponentTree(tree_path string) *config.ComponentTree {
	data, err := os.ReadFile(tree_path)
	if err != nil {
		panic(err)
	}

	component_tree := &config.ComponentTree{}
	err = protojson.Unmarshal(data, component_tree)
	if err != nil {
		panic(err)
	}

	return component_tree
	//fmt.Println("Extracted component tree:", protojson.Format(component_tree))
}
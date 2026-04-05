package main

import (
	"os"
	"sync"

	"helios/internal/client"
)

func main() {
	var wg sync.WaitGroup

	runtime_hash := os.Getenv("RUNTIME_HASH")
	docker_disabled := os.Getenv("DOCKER_DISABLED")
	component_tree_path := os.Getenv("COMPONENT_TREE_PATH")

	runType := "docker"
	if docker_disabled == "1" { runType = "local" }

	cli := client.Initialize(runType, runtime_hash)
	defer cli.Close()

	cli.InitializeComponentTree(component_tree_path)
	go cli.StartAllComponents()
	wg.Add(1)

	// Stop the main program from terminating
	// All code is now running in goroutines
	wg.Wait()
}

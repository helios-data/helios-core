package main

import (
	"fmt"
	"helios/internal/core"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	runtime_hash := os.Getenv("RUNTIME_HASH")
	docker_disabled := os.Getenv("DOCKER_DISABLED")
	component_tree_path := os.Getenv("COMPONENT_TREE_PATH")

	runType := "docker"
	if docker_disabled == "1" {
		runType = "local"
	}

	cli := core.Initialize(runType, runtime_hash)
	defer cli.Close()

	cli.InitializeComponentTree(component_tree_path)
	go cli.StartAllComponents()

	// Wait for Ctrl+C or kill signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // blocks until Ctrl+C or kill signal

	fmt.Println("Shutting down...")
}

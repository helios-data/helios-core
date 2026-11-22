//go:build !linux

package docker

import (
	"context"

	componenttree "helios/internal/component_tree"
)

// DeviceMonitor is a no-op on non-Linux platforms; device access is handled
// via --device mappings at container creation time.
type DeviceMonitor struct{}

func newDeviceMonitor(_ context.Context, _ *DockerClient, _ *componenttree.ComponentTree) *DeviceMonitor {
	return &DeviceMonitor{}
}

func (m *DeviceMonitor) start() error { return nil }
func (m *DeviceMonitor) stop()        {}

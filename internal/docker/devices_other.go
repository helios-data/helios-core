//go:build !linux

package docker

import (
	"github.com/docker/docker/api/types/container"
	configpb "helios/generated/config"
)

// buildDeviceConfig returns traditional DeviceMapping entries on non-Linux platforms.
func buildDeviceConfig(devices []*configpb.Device) (mappings []container.DeviceMapping, cgroupRules []string, extraBinds []string) {
	for _, device := range devices {
		mappings = append(mappings, container.DeviceMapping{
			PathOnHost:        device.Source,
			PathInContainer:   device.Target,
			CgroupPermissions: "rwm",
		})
	}
	return mappings, nil, nil
}

// applyInitialDeviceNodes is a no-op on non-Linux; --device handles it at container creation.
func (c *DockerClient) applyInitialDeviceNodes(containerID string, devices []*configpb.Device) {}

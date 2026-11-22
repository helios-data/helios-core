//go:build linux

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	configpb "helios/generated/config"
	"helios/internal/logger"

	"golang.org/x/sys/unix"
)

// buildDeviceConfig returns the device-related container config for Linux.
//
// Directory sources (e.g. /dev/snd): bind-mounted directly as a volume so the
// full directory is accessible and new nodes inside appear automatically.
//
// File/symlink sources (e.g. /dev/SRAD_Radio): cgroup rule added so the
// container can use whatever minor number the hot-plug monitor creates via mknod.
func buildDeviceConfig(devices []*configpb.Device) (mappings []container.DeviceMapping, cgroupRules []string, extraBinds []string) {
	if len(devices) == 0 {
		return nil, nil, nil
	}
	seenMajors := map[uint32]bool{}

	for _, device := range devices {
		info, err := os.Stat(device.Source)
		if err == nil && info.IsDir() {
			// Directory device: bind-mount and derive cgroup rules from its contents.
			extraBinds = append(extraBinds, fmt.Sprintf("%s:%s:rw", device.Source, device.Target))
			entries, _ := os.ReadDir(device.Source)
			for _, entry := range entries {
				major, _, err := resolveDeviceMajorMinor(filepath.Join(device.Source, entry.Name()))
				if err != nil {
					continue
				}
				if !seenMajors[major] {
					cgroupRules = append(cgroupRules, fmt.Sprintf("c %d:* rwm", major))
					seenMajors[major] = true
				}
			}
			// Fallback when the directory is empty at startup.
			if len(entries) == 0 {
				if major, err := inferMajorFromName(device.Source); err == nil && !seenMajors[major] {
					cgroupRules = append(cgroupRules, fmt.Sprintf("c %d:* rwm", major))
					seenMajors[major] = true
				}
			}
			continue
		}

		// File/symlink device: cgroup rule only; the hot-plug monitor handles node creation.
		major, err := resolveDeviceMajor(device.Source)
		if err != nil {
			logger.Warnw("Cannot determine device major for cgroup rule, skipping", "source", device.Source, "error", err)
			continue
		}
		if !seenMajors[major] {
			cgroupRules = append(cgroupRules, fmt.Sprintf("c %d:* rwm", major))
			seenMajors[major] = true
		}
	}
	return nil, cgroupRules, extraBinds
}

// applyInitialDeviceNodes creates device nodes inside a newly started container
// for file/symlink devices that already exist on the host.
// Directory devices are skipped — the bind mount covers them.
func (c *DockerClient) applyInitialDeviceNodes(containerID string, devices []*configpb.Device) {
	for _, device := range devices {
		info, err := os.Stat(device.Source)
		if err == nil && info.IsDir() {
			continue // handled by bind mount
		}
		major, minor, err := resolveDeviceMajorMinor(device.Source)
		if err != nil {
			continue
		}
		logger.Infow("Creating initial device node in container", "source", device.Source, "target", device.Target, "container", containerID)
		if err := c.execMknod(containerID, device.Target, major, minor); err != nil {
			logger.Warnw("Failed to create initial device node", "target", device.Target, "error", err)
		}
	}
}

// resolveDeviceMajorMinor stats path (following symlinks) and returns its major/minor numbers.
func resolveDeviceMajorMinor(path string) (major, minor uint32, err error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	var stat unix.Stat_t
	if err := unix.Stat(resolved, &stat); err != nil {
		return 0, 0, fmt.Errorf("stat %s: %w", resolved, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFCHR && stat.Mode&unix.S_IFMT != unix.S_IFBLK {
		return 0, 0, fmt.Errorf("%s is not a device file", resolved)
	}
	return unix.Major(stat.Rdev), unix.Minor(stat.Rdev), nil
}

// resolveDeviceMajor returns the major number, trying stat first then name-based lookup.
func resolveDeviceMajor(path string) (uint32, error) {
	major, _, err := resolveDeviceMajorMinor(path)
	if err == nil {
		return major, nil
	}
	return inferMajorFromName(path)
}

// inferMajorFromName maps common Linux device name patterns to major numbers.
func inferMajorFromName(path string) (uint32, error) {
	base := filepath.Base(path)
	if target, err := os.Readlink(path); err == nil {
		base = filepath.Base(target)
	}
	switch {
	case strings.HasPrefix(base, "ttyUSB"):
		return 188, nil
	case strings.HasPrefix(base, "ttyACM"):
		return 166, nil
	case strings.HasPrefix(base, "video"):
		return 81, nil
	case strings.HasPrefix(base, "snd"):
		return 116, nil
	default:
		return 0, fmt.Errorf("unknown device type for %q", base)
	}
}

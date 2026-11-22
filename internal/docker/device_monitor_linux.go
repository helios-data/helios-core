//go:build linux

package docker

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"unsafe"

	componenttree "helios/internal/component_tree"
	"helios/internal/logger"

	"golang.org/x/sys/unix"
)

// DeviceMonitor watches /dev via inotify and creates device nodes inside
// containers when file/symlink devices appear or change.
// Directory devices (e.g. /dev/snd) are handled by bind mounts instead.
type DeviceMonitor struct {
	ctx    context.Context
	cancel context.CancelFunc
	inoFd  int
	epFd   int
	client *DockerClient
	tree   *componenttree.ComponentTree
}

func newDeviceMonitor(ctx context.Context, client *DockerClient, tree *componenttree.ComponentTree) *DeviceMonitor {
	ctx, cancel := context.WithCancel(ctx)
	return &DeviceMonitor{
		ctx:    ctx,
		cancel: cancel,
		inoFd:  -1,
		epFd:   -1,
		client: client,
		tree:   tree,
	}
}

func (m *DeviceMonitor) start() error {
	inoFd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return fmt.Errorf("inotify_init1: %w", err)
	}

	if _, err := unix.InotifyAddWatch(inoFd, "/dev", unix.IN_CREATE|unix.IN_MOVED_TO); err != nil {
		unix.Close(inoFd)
		return fmt.Errorf("inotify_add_watch /dev: %w", err)
	}

	epFd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		unix.Close(inoFd)
		return fmt.Errorf("epoll_create1: %w", err)
	}

	ev := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(inoFd)}
	if err := unix.EpollCtl(epFd, unix.EPOLL_CTL_ADD, inoFd, &ev); err != nil {
		unix.Close(inoFd)
		unix.Close(epFd)
		return fmt.Errorf("epoll_ctl: %w", err)
	}

	m.inoFd = inoFd
	m.epFd = epFd

	go m.run()
	logger.Info("Device monitor started, watching /dev")
	return nil
}

func (m *DeviceMonitor) stop() {
	m.cancel()
	if m.epFd >= 0 {
		unix.Close(m.epFd)
		m.epFd = -1
	}
	if m.inoFd >= 0 {
		unix.Close(m.inoFd)
		m.inoFd = -1
	}
}

func (m *DeviceMonitor) run() {
	buf := make([]byte, 4096)
	epEvents := make([]unix.EpollEvent, 8)

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		n, err := unix.EpollWait(m.epFd, epEvents, 500)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		nread, err := unix.Read(m.inoFd, buf)
		if err != nil {
			if err == unix.EINTR || err == unix.EAGAIN {
				continue
			}
			return
		}

		m.parseEvents(buf[:nread])
	}
}

func (m *DeviceMonitor) parseEvents(buf []byte) {
	offset := 0
	for offset+unix.SizeofInotifyEvent <= len(buf) {
		event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
		nameLen := int(event.Len)
		name := ""
		if nameLen > 0 && offset+unix.SizeofInotifyEvent+nameLen <= len(buf) {
			raw := buf[offset+unix.SizeofInotifyEvent : offset+unix.SizeofInotifyEvent+nameLen]
			if i := bytes.IndexByte(raw, 0); i >= 0 {
				name = string(raw[:i])
			} else {
				name = string(raw)
			}
		}
		offset += unix.SizeofInotifyEvent + nameLen

		if name != "" {
			m.handleDeviceAppeared(filepath.Join("/dev", name))
		}
	}
}

func (m *DeviceMonitor) handleDeviceAppeared(hostPath string) {
	major, minor, err := resolveDeviceMajorMinor(hostPath)
	if err != nil {
		return
	}

	_ = m.tree.EachComponent(func(_ string, name string, comp *componenttree.Component) error {
		spec := comp.GetDockerSpec()
		if spec == nil {
			return nil
		}
		containerID := comp.GetDockerConnID()
		if containerID == "" {
			return nil
		}
		for _, device := range spec.Devices {
			if deviceMatchesHost(device.Source, hostPath) {
				logger.Infow("Hot-plug: device appeared, creating node in container",
					"device", hostPath, "container", name, "target", device.Target)
				if err := m.client.execMknod(containerID, device.Target, major, minor); err != nil {
					logger.Warnw("Failed to create hot-plug device node",
						"error", err, "container", name, "device", device.Target)
				}
			}
		}
		return nil
	})
}

// deviceMatchesHost reports whether source refers to hostPath, accounting for symlinks.
func deviceMatchesHost(source, hostPath string) bool {
	if source == hostPath {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(source); err == nil && resolved == hostPath {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(hostPath); err == nil && resolved == source {
		return true
	}
	return false
}

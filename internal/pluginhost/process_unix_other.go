//go:build unix && !linux

package pluginhost

import "syscall"

// hostSysProcAttr places the host in its own process group so retirement can
// signal every descendant. Parent-death signals are Linux-only.
func hostSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

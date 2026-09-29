//go:build linux

package pluginhost

import "syscall"

// hostSysProcAttr places the host in its own process group so retirement can
// signal every descendant, and asks the kernel to SIGKILL the host if the
// supervising thread dies so a crashed kernel cannot leave orphaned hosts.
// Linux ties the parent-death signal to the forking thread; the Go runtime
// only retires a thread when a goroutine exits while locked to it, which the
// Control server never does.
func hostSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

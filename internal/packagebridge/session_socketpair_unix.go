//go:build unix

package packagebridge

import "golang.org/x/sys/unix"

func closeSessionSocket(fd int) error {
	return unix.Close(fd)
}

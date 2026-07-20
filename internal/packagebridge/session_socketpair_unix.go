//go:build unix

package packagebridge

import "golang.org/x/sys/unix"

func closeSessionSocket(fd int) error {
	return unix.Close(fd)
}

func markSessionSocketpairCloseOnExec(fds [2]int) error {
	for _, fd := range fds {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil {
			return err
		}
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags|unix.FD_CLOEXEC); err != nil {
			return err
		}
	}
	return nil
}

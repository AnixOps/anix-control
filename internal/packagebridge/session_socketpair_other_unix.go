//go:build unix && !linux

package packagebridge

import "golang.org/x/sys/unix"

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

func newSessionSocketpair() ([2]int, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return [2]int{}, err
	}
	pair := [2]int{fds[0], fds[1]}
	if err := markSessionSocketpairCloseOnExec(pair); err != nil {
		_ = closeSessionSocket(pair[0])
		_ = closeSessionSocket(pair[1])
		return [2]int{}, err
	}
	return pair, nil
}

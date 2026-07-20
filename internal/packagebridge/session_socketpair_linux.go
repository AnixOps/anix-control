//go:build linux

package packagebridge

import "golang.org/x/sys/unix"

func newSessionSocketpair() ([2]int, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{fds[0], fds[1]}, nil
}

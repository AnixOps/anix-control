//go:build unix && !linux

package packagebridge

import "golang.org/x/sys/unix"

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

//go:build !unix

package packagebridge

import "errors"

func newSessionSocketpair() ([2]int, error) {
	return [2]int{}, errors.New("package bridge sessions require Unix socketpair support")
}

func closeSessionSocket(int) error {
	return nil
}

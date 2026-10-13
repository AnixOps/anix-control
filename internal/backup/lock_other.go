//go:build !unix

package backup

import (
	"errors"
	"os"
)

// Lock is a held lock; without flock there is none.
type Lock struct{}

// Release is a no-op.
func (l *Lock) Release() {}

// ErrServiceRunning: see lock_unix.go.
var ErrServiceRunning = errors.New("the AnixOps Control service is running on this data directory: stop it first")

// ErrRestoreRunning: see lock_unix.go.
var ErrRestoreRunning = errors.New("a backup restore is running on this data directory")

// LockService is a no-op without flock.
func LockService(string) (*Lock, error) { return &Lock{}, nil }

// LockExclusive is a no-op without flock.
func LockExclusive(string) (*Lock, error) { return &Lock{}, nil }

func openShared(path string) (*os.File, error) { return os.Open(path) } // #nosec G304 -- a bundle.

func removeUnlessInUse(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func databaseInUse(string) (bool, error) { return false, nil }

func freeBytes(string) (uint64, bool) { return 0, false }

func ownTo(string) error { return nil }

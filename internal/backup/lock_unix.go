//go:build unix

package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Lock is a held lock on DataDir/control.lock, or on a bundle.
type Lock struct {
	file *os.File
}

// Release lets the lock go. A nil Lock is a no-op.
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN) // #nosec G115 -- a file descriptor fits an int.
	_ = l.file.Close()
	l.file = nil
}

// ErrServiceRunning: the exclusive lock is held elsewhere; Control (or
// another restore) uses the data directory.
var ErrServiceRunning = errors.New("the AnixOps Control service is running on this data directory (it holds " + LockFileName + "): stop it first, for example with `sudo systemctl stop anix-control`, then run the command again")

// ErrRestoreRunning: a restore holds the data directory.
var ErrRestoreRunning = errors.New("a backup restore is running on this data directory (it holds " + LockFileName + " exclusively): wait until it finished")

// LockService takes the lock a running Control holds for its whole life:
// a shared flock on DataDir/control.lock. Several Control processes may
// hold it at once; a restore cannot take it while any of them does, and
// none can take it while a restore runs (ErrRestoreRunning). Its holder
// keeps the returned Lock until it exits; the kernel drops the lock when
// the process ends, however it ends. A data directory that does not exist
// yet, or that this process cannot write, has nothing to protect: the Lock
// is then a no-op.
func LockService(dataDir string) (*Lock, error) {
	if _, err := os.Stat(dataDir); errors.Is(err, os.ErrNotExist) {
		return &Lock{}, nil
	}
	lock, err := lockFile(filepath.Join(dataDir, LockFileName), unix.LOCK_SH, ErrRestoreRunning)
	if err != nil && lockUnavailable(err) {
		// A process that cannot create the file cannot be running on a
		// data directory a restore could change either: a container with a
		// read-only root file system and no volume at data_dir (the Compose
		// migrate service), or a data directory owned by another user. It
		// has nothing to protect and nothing to refuse.
		return &Lock{}, nil
	}
	return lock, err
}

// lockUnavailable reports an error of creating the lock file that means the
// directory cannot be written by this process.
func lockUnavailable(err error) bool {
	return errors.Is(err, unix.EROFS) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM)
}

// LockExclusive takes DataDir/control.lock exclusively, without waiting:
// a restore holds it while it changes the data directory. It answers
// ErrServiceRunning while a Control process holds the lock.
func LockExclusive(dataDir string) (*Lock, error) {
	return lockFile(filepath.Join(dataDir, LockFileName), unix.LOCK_EX, ErrServiceRunning)
}

// lockFileMode: the lock file holds nothing; everyone who may run Control
// on the data directory must be able to open it, whoever created it.
const lockFileMode os.FileMode = 0o644

func lockFile(path string, how int, busy error) (*Lock, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, lockFileMode) // #nosec G304,G302 -- the lock file of the data directory; it holds nothing.
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	_ = ownTo(path)
	if err := unix.Flock(int(file.Fd()), how|unix.LOCK_NB); err != nil { // #nosec G115 -- a file descriptor fits an int.
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, busy
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return &Lock{file: file}, nil
}

// openShared opens a bundle for reading with a shared flock: a bundle being
// read (verified, downloaded, restored) is never removed by the retention,
// which takes the lock exclusively before it removes a bundle.
func openShared(path string) (*os.File, error) {
	file, err := os.Open(path) // #nosec G304 -- a bundle of the backup directory or one the operator named.
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_SH); err != nil { // #nosec G115 -- a file descriptor fits an int.
		_ = file.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return file, nil
}

// removeUnlessInUse removes a bundle unless someone reads it: it takes the
// bundle's flock exclusively without waiting and removes the file while it
// holds the lock. removed is false when the bundle is in use.
func removeUnlessInUse(path string) (removed bool, err error) {
	file, err := os.Open(path) // #nosec G304 -- a bundle of the backup directory.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = file.Close() }()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil { // #nosec G115 -- a file descriptor fits an int.
		if errors.Is(err, unix.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

// sqliteSharedRange is where SQLite's unix VFS takes the SHARED lock of a
// database file (PENDING_BYTE + 2, 510 bytes). Every connection that has a
// WAL database open holds a read lock there, idle or not.
const (
	sqlitePendingByte  = 0x40000000
	sqliteSharedFirst  = sqlitePendingByte + 2
	sqliteSharedLength = 510
)

// databaseInUse reports whether another process has the SQLite database at
// path open: it asks the kernel whether a write lock on the SHARED range of
// the file, or on its -shm file, would conflict with a lock someone holds.
// It changes nothing (F_GETLK). A lock this process holds is not seen.
func databaseInUse(path string) (bool, error) {
	for _, candidate := range []struct {
		path          string
		start, length int64
	}{
		{path, sqliteSharedFirst, sqliteSharedLength},
		{path + "-shm", 0, 0},
	} {
		file, err := os.OpenFile(candidate.path, os.O_RDWR, 0) // #nosec G304 -- the database the restore replaces.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		probe := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: candidate.start, Len: candidate.length}
		err = unix.FcntlFlock(file.Fd(), unix.F_GETLK, &probe)
		_ = file.Close()
		if err != nil {
			return false, err
		}
		if probe.Type != unix.F_UNLCK {
			return true, nil
		}
	}
	return false, nil
}

// freeBytes answers the space an unprivileged user may still use on the
// file system of dir.
func freeBytes(dir string) (uint64, bool) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0, false
	}
	return stat.Bavail * uint64(stat.Bsize), true // #nosec G115 -- the block size is positive.
}

// ownTo gives path the owner of its directory, when this process runs as
// root: a restore run with sudo writes files the service user must read.
func ownTo(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	var stat unix.Stat_t
	if err := unix.Stat(filepath.Dir(path), &stat); err != nil {
		return err
	}
	return os.Lchown(path, int(stat.Uid), int(stat.Gid))
}

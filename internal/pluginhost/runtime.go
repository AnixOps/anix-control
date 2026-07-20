//go:build unix

package pluginhost

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	childRuntimeDirectoryFD   = 3
	childRuntimeDirectoryPath = "/proc/self/fd/3"
	maxHostRuntimeDirAttempts = 16
)

// runtimeRoot owns the descriptor used for every host runtime operation. The
// configured pathname is retained only for diagnostics; no host operation
// resolves it after this descriptor has been opened.
type runtimeRoot struct {
	directory      *os.File
	configuredPath string
	cleanupParent  *os.File
	cleanupName    string
}

// hostRuntimeDir is a private child directory created relative to a pinned
// runtimeRoot descriptor. The child process receives directory as FD 3.
type hostRuntimeDir struct {
	root      *runtimeRoot
	directory *os.File
	name      string
}

func newRuntimeRoot(configuredPath string) (*runtimeRoot, error) {
	if strings.TrimSpace(configuredPath) == "" {
		return newDefaultRuntimeRoot()
	}
	if !filepath.IsAbs(configuredPath) {
		return nil, fmt.Errorf("%w: host runtime directory must be absolute", ErrHostIncompatible)
	}
	configuredPath = filepath.Clean(configuredPath)
	if configuredPath == string(filepath.Separator) {
		return nil, fmt.Errorf("%w: host runtime directory must not be the filesystem root", ErrHostIncompatible)
	}
	return openConfiguredRuntimeRoot(configuredPath)
}

func newDefaultRuntimeRoot() (*runtimeRoot, error) {
	directoryPath, err := os.MkdirTemp("", "anixops-plugin-hosts-")
	if err != nil {
		return nil, fmt.Errorf("%w: create private host runtime directory: %v", ErrHostUnavailable, err)
	}
	directory, err := openDirectoryNoFollow(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("%w: open private host runtime directory: %v", ErrHostIncompatible, err)
	}
	if err := secureRuntimeDirectory(directory, true); err != nil {
		_ = directory.Close()
		return nil, err
	}
	parent, err := openDirectoryNoFollow(filepath.Dir(directoryPath))
	if err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("%w: open private host runtime parent: %v", ErrHostUnavailable, err)
	}
	return &runtimeRoot{
		directory: directory, configuredPath: directoryPath,
		cleanupParent: parent, cleanupName: filepath.Base(directoryPath),
	}, nil
}

func openConfiguredRuntimeRoot(configuredPath string) (*runtimeRoot, error) {
	current, err := openDirectoryNoFollow(string(filepath.Separator))
	if err != nil {
		return nil, fmt.Errorf("%w: open filesystem root: %v", ErrHostUnavailable, err)
	}
	components := strings.Split(strings.TrimPrefix(configuredPath, string(filepath.Separator)), string(filepath.Separator))
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = current.Close()
			return nil, fmt.Errorf("%w: host runtime directory is invalid", ErrHostIncompatible)
		}
		if err := validateTrustedRuntimeAncestor(current); err != nil {
			_ = current.Close()
			return nil, err
		}

		next, err := openOrCreateRuntimeDirectory(current, component)
		if err != nil {
			_ = current.Close()
			return nil, err
		}
		if err := current.Close(); err != nil {
			_ = next.Close()
			return nil, fmt.Errorf("%w: close host runtime directory ancestor: %v", ErrHostUnavailable, err)
		}
		current = next

		if index == len(components)-1 {
			if err := secureRuntimeDirectory(current, true); err != nil {
				_ = current.Close()
				return nil, err
			}
		}
	}
	return &runtimeRoot{directory: current, configuredPath: configuredPath}, nil
}

func openOrCreateRuntimeDirectory(parent *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(directoryFD(parent), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		if mkdirErr := unix.Mkdirat(directoryFD(parent), name, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
			return nil, fmt.Errorf("%w: create host runtime directory: %v", ErrHostUnavailable, mkdirErr)
		}
		fd, err = unix.Openat(directoryFD(parent), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: open host runtime directory without following links: %v", ErrHostIncompatible, err)
	}
	directory := os.NewFile(uintptr(fd), name)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: open host runtime directory", ErrHostUnavailable)
	}
	return directory, nil
}

func openDirectoryNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(fd), path)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, unix.EBADF
	}
	return directory, nil
}

func validateTrustedRuntimeAncestor(directory *os.File) error {
	stat, err := directoryStat(directory)
	if err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%w: host runtime directory ancestor is not a directory", ErrHostIncompatible)
	}
	if stat.Mode&(unix.S_IWGRP|unix.S_IWOTH) != 0 {
		return fmt.Errorf("%w: host runtime directory ancestor is writable by group or others", ErrHostIncompatible)
	}
	if int(stat.Uid) != os.Geteuid() && stat.Uid != 0 {
		return fmt.Errorf("%w: host runtime directory ancestor is not trusted", ErrHostIncompatible)
	}
	return nil
}

func secureRuntimeDirectory(directory *os.File, requireOwner bool) error {
	stat, err := directoryStat(directory)
	if err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%w: host runtime directory is not a directory", ErrHostIncompatible)
	}
	if requireOwner && int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: host runtime directory is not owned by the service user", ErrHostIncompatible)
	}
	if err := unix.Fchmod(directoryFD(directory), 0o700); err != nil {
		return fmt.Errorf("%w: secure host runtime directory: %v", ErrHostUnavailable, err)
	}
	stat, err = directoryStat(directory)
	if err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o7777 != 0o700 {
		return fmt.Errorf("%w: host runtime directory is not private", ErrHostIncompatible)
	}
	return nil
}

func directoryStat(directory *os.File) (unix.Stat_t, error) {
	if directory == nil {
		return unix.Stat_t{}, fmt.Errorf("%w: host runtime directory is unavailable", ErrHostUnavailable)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(directoryFD(directory), &stat); err != nil {
		return unix.Stat_t{}, fmt.Errorf("%w: inspect host runtime directory: %v", ErrHostUnavailable, err)
	}
	return stat, nil
}

func (root *runtimeRoot) createHostRuntimeDir() (*hostRuntimeDir, error) {
	if root == nil || root.directory == nil {
		return nil, fmt.Errorf("%w: host runtime directory is unavailable", ErrHostUnavailable)
	}
	for attempt := 0; attempt < maxHostRuntimeDirAttempts; attempt++ {
		name, err := randomHostRuntimeDirName()
		if err != nil {
			return nil, err
		}
		if err := unix.Mkdirat(directoryFD(root.directory), name, 0o700); err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			return nil, fmt.Errorf("%w: create package runtime directory: %v", ErrHostUnavailable, err)
		}
		fd, err := unix.Openat(directoryFD(root.directory), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			_ = unix.Unlinkat(directoryFD(root.directory), name, unix.AT_REMOVEDIR)
			return nil, fmt.Errorf("%w: open package runtime directory: %v", ErrHostIncompatible, err)
		}
		directory := os.NewFile(uintptr(fd), name)
		if directory == nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(directoryFD(root.directory), name, unix.AT_REMOVEDIR)
			return nil, fmt.Errorf("%w: open package runtime directory", ErrHostUnavailable)
		}
		if err := secureRuntimeDirectory(directory, true); err != nil {
			_ = directory.Close()
			_ = unix.Unlinkat(directoryFD(root.directory), name, unix.AT_REMOVEDIR)
			return nil, err
		}
		return &hostRuntimeDir{root: root, directory: directory, name: name}, nil
	}
	return nil, fmt.Errorf("%w: create unique package runtime directory", ErrHostUnavailable)
}

func randomHostRuntimeDirName() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("%w: create package runtime directory name: %v", ErrHostUnavailable, err)
	}
	return "host-" + hex.EncodeToString(value), nil
}

func (root *runtimeRoot) Close() error {
	if root == nil {
		return nil
	}
	var closeErr error
	if root.directory != nil {
		closeErr = errors.Join(closeErr, root.directory.Close())
		root.directory = nil
	}
	if root.cleanupParent != nil {
		if root.cleanupName != "" {
			if err := unix.Unlinkat(directoryFD(root.cleanupParent), root.cleanupName, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
				closeErr = errors.Join(closeErr, fmt.Errorf("remove private host runtime directory: %w", err))
			}
		}
		closeErr = errors.Join(closeErr, root.cleanupParent.Close())
		root.cleanupParent = nil
	}
	return closeErr
}

func (directory *hostRuntimeDir) path(name string) string {
	if directory == nil || directory.directory == nil {
		return ""
	}
	base := "/proc/self/fd/" + strconv.FormatUint(uint64(directory.directory.Fd()), 10)
	if name == "" || name == "." {
		return base
	}
	return base + "/" + name
}

func childRuntimePath(name string) string {
	if name == "" || name == "." {
		return childRuntimeDirectoryPath
	}
	return childRuntimeDirectoryPath + "/" + name
}

func (directory *hostRuntimeDir) remove() error {
	if directory == nil {
		return nil
	}
	var removeErr error
	if directory.directory != nil {
		removeErr = errors.Join(removeErr, removeDirectoryContents(directory.directory))
		removeErr = errors.Join(removeErr, directory.directory.Close())
		directory.directory = nil
	}
	if directory.root != nil && directory.root.directory != nil && directory.name != "" {
		if err := unix.Unlinkat(directoryFD(directory.root.directory), directory.name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
			removeErr = errors.Join(removeErr, fmt.Errorf("remove package runtime directory: %w", err))
		}
	}
	return removeErr
}

func removeDirectoryContents(directory *os.File) error {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read package runtime directory: %w", err)
	}
	var removeErr error
	for _, entry := range entries {
		name := entry.Name()
		childFD, openErr := unix.Openat(directoryFD(directory), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr == nil {
			child := os.NewFile(uintptr(childFD), name)
			if child == nil {
				_ = unix.Close(childFD)
				removeErr = errors.Join(removeErr, fmt.Errorf("open nested package runtime directory"))
				continue
			}
			removeErr = errors.Join(removeErr, removeDirectoryContents(child), child.Close())
			if err := unix.Unlinkat(directoryFD(directory), name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
				removeErr = errors.Join(removeErr, fmt.Errorf("remove nested package runtime directory: %w", err))
			}
			continue
		}
		if err := unix.Unlinkat(directoryFD(directory), name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			removeErr = errors.Join(removeErr, fmt.Errorf("remove package runtime file: %w", err))
		}
	}
	return removeErr
}

func directoryFD(directory *os.File) int {
	return int(directory.Fd())
}

//go:build unix

package pluginhost

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
)

// BuiltinHostCommand is the Control subcommand that runs a package host
// linked into the Control binary: `anix-control package-host <id>`
// (packages/builtin.CommandName, cmd/server).
const BuiltinHostCommand = "package-host"

// BuiltinHostVersionEnvironment passes a built-in host the release version
// the kernel verified (packages/builtin.VersionEnvironment).
const BuiltinHostVersionEnvironment = "ANIX_CONTROL_HOST_PACKAGE_VERSION"

// builtinHostArgv0 is the process name a built-in host shows in ps.
const builtinHostArgv0 = "anix-control"

// SelfExecutable is the identity of the running Control binary, recorded
// once, before the first built-in host starts. A built-in host executes this
// file and nothing else, so its trust is the binary's own: the release signs
// SHA256SUMS.txt (SHA256SUMS.txt.sig, the official package key) and the
// installer verifies the archive the binary comes from against it.
//
// On Linux the host is started through /proc/self/exe. The kernel resolves
// that link in the new process to the image the parent is running, so a
// host always runs the parent's bytes, even when an upgrade has already
// replaced the file at the install path. Before every start the launcher
// checks that /proc/self/exe still names the recorded file (device, inode,
// size and modification time): a file that changed in place is refused.
// Elsewhere (development machines) it executes os.Executable() and hashes
// the file again before every start.
type SelfExecutable struct {
	// Path is what a host start executes.
	Path string
	// SHA256 is the binary's digest when it was recorded.
	SHA256 string
	// Pinned reports /proc/self/exe: the path cannot name another file.
	Pinned bool

	device  uint64
	inode   uint64
	size    int64
	modTime int64
}

var (
	selfOnce  sync.Once
	selfValue SelfExecutable
	selfErr   error
)

// Self records the running binary's identity on first use and returns it.
func Self() (SelfExecutable, error) {
	selfOnce.Do(func() {
		selfValue, selfErr = recordSelfExecutable()
	})
	return selfValue, selfErr
}

func recordSelfExecutable() (SelfExecutable, error) {
	path, pinned := "/proc/self/exe", true
	if runtime.GOOS != "linux" {
		resolved, err := os.Executable()
		if err != nil {
			return SelfExecutable{}, fmt.Errorf("%w: locate the Control executable: %v", ErrHostIncompatible, err)
		}
		path, pinned = resolved, false
	}
	identity, digest, err := statAndHashExecutable(path)
	if err != nil {
		return SelfExecutable{}, err
	}
	identity.Path, identity.SHA256, identity.Pinned = path, digest, pinned
	return identity, nil
}

func statAndHashExecutable(path string) (SelfExecutable, string, error) {
	file, err := os.Open(path) // #nosec G304 -- the running binary: /proc/self/exe or os.Executable().
	if err != nil {
		return SelfExecutable{}, "", fmt.Errorf("%w: open the Control executable: %v", ErrHostIncompatible, err)
	}
	defer func() { _ = file.Close() }()
	identity, err := executableIdentity(file)
	if err != nil {
		return SelfExecutable{}, "", err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return SelfExecutable{}, "", fmt.Errorf("%w: read the Control executable: %v", ErrHostIncompatible, err)
	}
	return identity, hex.EncodeToString(hasher.Sum(nil)), nil
}

func executableIdentity(file *os.File) (SelfExecutable, error) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return SelfExecutable{}, fmt.Errorf("%w: the Control executable is not a regular file", ErrHostIncompatible)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return SelfExecutable{}, fmt.Errorf("%w: the Control executable is not executable", ErrHostIncompatible)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return SelfExecutable{}, fmt.Errorf("%w: the Control executable has no file identity", ErrHostIncompatible)
	}
	return SelfExecutable{
		device: uint64(stat.Dev), inode: uint64(stat.Ino), // #nosec G115 -- device and inode numbers are opaque identifiers.
		size: info.Size(), modTime: info.ModTime().UnixNano(),
	}, nil
}

// Check verifies that Path still names the recorded binary.
func (s SelfExecutable) Check() error {
	if s.Path == "" || s.SHA256 == "" {
		return fmt.Errorf("%w: the Control executable was not recorded", ErrHostIncompatible)
	}
	if s.Pinned {
		file, err := os.Open(s.Path)
		if err != nil {
			return fmt.Errorf("%w: open the Control executable: %v", ErrHostIncompatible, err)
		}
		defer func() { _ = file.Close() }()
		current, err := executableIdentity(file)
		if err != nil {
			return err
		}
		if !s.sameFile(current) {
			return fmt.Errorf("%w: the Control executable changed since it started", ErrHostIncompatible)
		}
		return nil
	}
	current, digest, err := statAndHashExecutable(s.Path)
	if err != nil {
		return err
	}
	expected, _ := hex.DecodeString(s.SHA256)
	actual, _ := hex.DecodeString(digest)
	if !s.sameFile(current) || subtle.ConstantTimeCompare(expected, actual) != 1 {
		return fmt.Errorf("%w: the Control executable changed since it started", ErrHostIncompatible)
	}
	return nil
}

func (s SelfExecutable) sameFile(other SelfExecutable) bool {
	return s.device == other.device && s.inode == other.inode && s.size == other.size && s.modTime == other.modTime
}

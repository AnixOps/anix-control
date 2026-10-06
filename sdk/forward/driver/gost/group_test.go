package gost

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// gost may run as its own user: the configuration the Agent writes takes the
// group of its directory, while the Agent's own group-private state does not.
func TestGroupReadableFilesTakeTheirDirectorysGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing a file's group to one the process is not in needs root")
	}
	dir := t.TempDir()
	const gostGroup = 31337
	if err := os.Chown(dir, 0, gostGroup); err != nil {
		t.Fatal(err)
	}
	group := func(name string) int {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return int(info.Sys().(*syscall.Stat_t).Gid)
	}
	if err := writeFileAtomic(filepath.Join(dir, "gost.json"), []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(filepath.Join(dir, "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if g := group("gost.json"); g != gostGroup {
		t.Fatalf("gost.json has group %d, want the directory's %d", g, gostGroup)
	}
	if g := group("state.json"); g == gostGroup {
		t.Fatalf("state.json is group-private, but has the directory's group %d", g)
	}
}

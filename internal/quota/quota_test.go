package quota

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestAllowWithoutACheckAllowsEverything(t *testing.T) {
	for _, kind := range []Kind{Users, Nodes} {
		if err := Allow(nil, kind); err != nil {
			t.Fatalf("Allow(%s) = %v, want nil", kind, err)
		}
	}
}

func TestAllowReturnsTheRefusal(t *testing.T) {
	Check = func(_ *gorm.DB, kind Kind) error { return fmt.Errorf("%w: %s", ErrExceeded, kind) }
	t.Cleanup(func() { Check = nil })
	if err := Allow(nil, Nodes); !errors.Is(err, ErrExceeded) {
		t.Fatalf("Allow(nodes) = %v, want ErrExceeded", err)
	}
}

// TestNoPublicCodeSetsTheCheck keeps the community edition unlimited: only
// tests in this repository assign quota.Check; the commercial edition sets it
// from its own files.
func TestNoPublicCodeSetsTheCheck(t *testing.T) {
	assign := regexp.MustCompile(`\bquota\.Check\s*=[^=]`)
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if assign.Match(source) {
			t.Errorf("%s sets quota.Check; the community edition must not limit creations", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

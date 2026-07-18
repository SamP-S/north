package board

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStealStaleRemovesStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LockName)
	if err := os.WriteFile(path, []byte("pid 1 at long ago\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * staleAfter)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	stealStale(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stale lock should be gone after steal, stat: %v", err)
	}
	if leftovers, _ := filepath.Glob(path + ".steal.*"); len(leftovers) != 0 {
		t.Errorf("steal temp files left behind: %v", leftovers)
	}
}

func TestStealStaleRestoresFresh(t *testing.T) {
	// A lock that turns out fresh at re-verify time — another waiter stole
	// and recreated it between the caller's stat and the rename — must be
	// restored, not destroyed.
	dir := t.TempDir()
	path := filepath.Join(dir, LockName)
	const content = "pid 42 fresh\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stealStale(path)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != content {
		t.Fatalf("fresh lock should be restored intact, got %q (%v)", data, err)
	}
	if leftovers, _ := filepath.Glob(path + ".steal.*"); len(leftovers) != 0 {
		t.Errorf("steal temp files left behind: %v", leftovers)
	}
}

package board_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/board"
	nerrors "github.com/SamP-S/north/internal/errors"
)

func TestLockAcquireRelease(t *testing.T) {
	boardDir := newBoard(t)
	release, err := board.Lock(boardDir)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	lockPath := filepath.Join(boardDir, board.LockName)
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file missing while held: %v", err)
	}
	release()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed on release")
	}
	// Reacquirable after release.
	release2, err := board.Lock(boardDir)
	if err != nil {
		t.Fatalf("relock: %v", err)
	}
	release2()
}

func TestLockWaitsForHolder(t *testing.T) {
	boardDir := newBoard(t)
	release, err := board.Lock(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	// Release shortly after the second acquire starts; it must win via retry.
	go func() {
		time.Sleep(100 * time.Millisecond)
		release()
	}()
	start := time.Now()
	release2, err := board.Lock(boardDir)
	if err != nil {
		t.Fatalf("lock should succeed once the holder releases: %v", err)
	}
	release2()
	if time.Since(start) < 50*time.Millisecond {
		t.Error("second lock acquired while the first was still held")
	}
}

func TestLockStealsStale(t *testing.T) {
	boardDir := newBoard(t)
	lockPath := filepath.Join(boardDir, board.LockName)
	if err := os.WriteFile(lockPath, []byte("pid 1 at long ago\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	release, err := board.Lock(boardDir)
	if err != nil {
		t.Fatalf("stale lock should be stolen: %v", err)
	}
	release()
}

func TestLockStealRace(t *testing.T) {
	boardDir := newBoard(t)
	lockPath := filepath.Join(boardDir, board.LockName)
	if err := os.WriteFile(lockPath, []byte("pid 1 at long ago\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	// Two waiters race to steal the same stale lock. Only one rename can
	// succeed; the loser's failed steal must not be an error — it keeps
	// retrying and acquires once the winner releases.
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			release, err := board.Lock(boardDir)
			if err == nil {
				time.Sleep(30 * time.Millisecond)
				release()
			}
			errs <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("lock during steal race: %v", err)
		}
	}
	if leftovers, _ := filepath.Glob(lockPath + ".steal.*"); len(leftovers) != 0 {
		t.Errorf("steal temp files left behind: %v", leftovers)
	}
}

func TestLockConflictWhenHeld(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the lock budget")
	}
	boardDir := newBoard(t)
	release, err := board.Lock(boardDir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = board.Lock(boardDir)
	if be, ok := nerrors.As(err); !ok || be.Code() != "conflict" {
		t.Fatalf("expected conflict while held, got %v", err)
	}
}

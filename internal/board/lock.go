// lock.go — advisory file lock serialising board mutations.
//
// The lock is a "north/.lock" file created with O_CREATE|O_EXCL and removed on
// release. It guards the read-allocate-write window of mutating operations
// (notably the NextID scan), so it is held only for the duration of one
// mutation. A lock file older than staleAfter is presumed abandoned by a
// crashed process and is stolen. Stdlib only — no flock, so it behaves the
// same on every platform git works on.
package board

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SamP-S/north/internal/errors"
)

// LockName is the lock file's name inside the board dir.
const LockName = ".lock"

const (
	lockRetry  = 25 * time.Millisecond
	lockWait   = 2 * time.Second
	staleAfter = 10 * time.Second
)

// Lock takes the advisory board lock and returns a release func. It retries
// briefly while another process holds the lock, steals locks older than
// staleAfter, and returns Conflict when the board stays locked past the wait
// budget.
func Lock(boardDir string) (func(), error) {
	path := filepath.Join(boardDir, LockName)
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			fmt.Fprintf(f, "pid %d at %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if info, serr := os.Stat(path); serr == nil && time.Since(info.ModTime()) > staleAfter {
			// Holder is presumed crashed; remove and race to recreate.
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, errors.Conflict(fmt.Sprintf(
				"board is locked by another north process (%s); retry, or delete the file if stale", path))
		}
		time.Sleep(lockRetry)
	}
}

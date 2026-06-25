// Package git provides an optional local git commit for board mutations.
//
// Only used when auto_commit: true in "north/config.yml". North never pushes or
// pulls — remote git is entirely the user's concern. Best-effort: if the board
// is not inside a git repo, committing is silently skipped.
package git

import (
	"log"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
)

// CommitBoard stages the given paths (and removals) and commits them locally.
// A no-op when the board is not in a git work tree.
func CommitBoard(board, message string, paths, removed []string) error {
	repo, err := gogit.PlainOpenWithOptions(board, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		log.Printf("auto_commit is on but %s is not in a git repo; skipping commit", board)
		return nil
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	root := wt.Filesystem.Root()

	staged := false
	// Add records additions, modifications, and deletions (when the file is
	// gone from disk), covering both `paths` and `removed`.
	for _, p := range append(append([]string{}, paths...), removed...) {
		rel, err := filepath.Rel(root, mustAbs(p))
		if err != nil {
			continue
		}
		if _, err := wt.Add(rel); err != nil {
			return err
		}
		staged = true
	}
	if !staged {
		return nil
	}
	if _, err := wt.Commit(message, &gogit.CommitOptions{}); err != nil && err != gogit.ErrEmptyCommit {
		return err
	}
	return nil
}

func mustAbs(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Package git provides an optional local git commit for board mutations by
// shelling out to the system git binary.
//
// Only used when auto_commit: true in "north/config.yml". North never pushes or
// pulls — remote git is entirely the user's concern. Using the real git binary
// (rather than a reimplementation) means linked worktrees, hooks, commit
// signing, and includeIf config all behave exactly as the user's git does. If
// the board is not inside a git work tree, committing is silently skipped; if
// git itself is missing while auto_commit is on, that is a real error.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CommitBoard stages the given paths (and removals) and commits them locally.
// A no-op when the board is not in a git work tree. Only the given paths are
// committed — anything else the user has staged is left untouched. When no git
// identity is configured, a fallback "north" identity is used so the commit
// never fails half-way through a mutation.
func CommitBoard(board, message string, paths, removed []string) error {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("auto_commit is on but git is not installed")
	}
	if !insideWorkTree(gitBin, board) {
		fmt.Fprintf(os.Stderr, "warning: auto_commit is on but %s is not in a git repo; skipping commit\n", board)
		return nil
	}

	all := stageable(gitBin, board, append(append([]string{}, paths...), removed...))
	if len(all) == 0 {
		return nil
	}

	// Stage additions, modifications, and deletions for exactly our paths.
	if _, err := run(gitBin, board, append([]string{"add", "-A", "--"}, all...)...); err != nil {
		return err
	}
	// Nothing actually changed → nothing to commit.
	if _, err := run(gitBin, board, append([]string{"diff", "--cached", "--quiet", "--"}, all...)...); err == nil {
		return nil
	}

	args := identityArgs(gitBin, board)
	args = append(args, "commit", "-m", message, "--")
	args = append(args, all...)
	if _, err := run(gitBin, board, args...); err != nil {
		return err
	}
	return nil
}

// stageable filters paths down to what `git add` can act on: paths that still
// exist on disk, plus deleted paths git tracks (whose removal must be staged).
// A deleted-and-untracked path — e.g. deleting a task created before
// auto_commit was enabled — would fail `git add` with an unmatched pathspec
// even though there is nothing to record.
func stageable(gitBin, dir string, paths []string) []string {
	var out []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
			continue
		}
		if _, err := run(gitBin, dir, "ls-files", "--error-unmatch", "--", p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// identityArgs returns -c user.name/user.email fallbacks when the user has no
// git identity configured (so auto_commit works out of the box), or nil when
// their own identity is set.
func identityArgs(gitBin, dir string) []string {
	name, _ := run(gitBin, dir, "config", "user.name")
	email, _ := run(gitBin, dir, "config", "user.email")
	if strings.TrimSpace(name) != "" && strings.TrimSpace(email) != "" {
		return nil
	}
	return []string{"-c", "user.name=north", "-c", "user.email=north@localhost"}
}

// insideWorkTree reports whether dir is inside a git work tree (including
// linked worktrees, where .git is a file).
func insideWorkTree(gitBin, dir string) bool {
	out, err := run(gitBin, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// run executes git -C dir args…, returning stdout. Errors include stderr.
func run(gitBin, dir string, args ...string) (string, error) {
	cmd := exec.Command(gitBin, append([]string{"-C", dir}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

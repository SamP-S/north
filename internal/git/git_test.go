package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/SamP-S/north/internal/git"
)

// runGit executes git -C dir args…, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// isolateGitConfig shields every git invocation in the test — the helpers
// here and the ones CommitBoard itself runs — from the developer's
// global/system git config.
func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func initRepo(t *testing.T) string {
	t.Helper()
	isolateGitConfig(t)
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "tester")
	runGit(t, root, "config", "user.email", "tester@example.com")
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func commitCount(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return 0 // no commits yet
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

// TestCommitBoard_NotInGitRepo confirms CommitBoard is a silent no-op when
// the board directory is not inside a git work tree.
func TestCommitBoard_NotInGitRepo(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	if err := git.CommitBoard(dir, "test", []string{}, nil); err != nil {
		t.Errorf("expected nil for non-repo, got %v", err)
	}
}

// TestCommitBoard_NoPaths confirms that an empty path list produces no commit.
func TestCommitBoard_NoPaths(t *testing.T) {
	root := initRepo(t)
	if err := git.CommitBoard(root, "north: test", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := commitCount(t, root); n != 0 {
		t.Errorf("expected 0 commits for empty paths, got %d", n)
	}
}

// TestCommitBoard_StagesAndCommits confirms that a new file is staged and
// committed with the supplied message.
func TestCommitBoard_StagesAndCommits(t *testing.T) {
	root := initRepo(t)
	p := filepath.Join(root, "1-task.md")
	writeFile(t, p, "# task 1")

	if err := git.CommitBoard(root, "north: create 1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}
	if n := commitCount(t, root); n != 1 {
		t.Errorf("expected 1 commit, got %d", n)
	}
	if msg := runGit(t, root, "log", "-1", "--format=%s"); msg != "north: create 1" {
		t.Errorf("unexpected commit message %q", msg)
	}
}

// TestCommitBoard_MultipleFiles confirms that several paths are staged in a
// single commit.
func TestCommitBoard_MultipleFiles(t *testing.T) {
	root := initRepo(t)
	paths := []string{
		filepath.Join(root, "1-task.md"),
		filepath.Join(root, "2-task.md"),
	}
	for _, p := range paths {
		writeFile(t, p, "# "+filepath.Base(p))
	}

	if err := git.CommitBoard(root, "north: batch", paths, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}
	if n := commitCount(t, root); n != 1 {
		t.Errorf("expected 1 commit for multiple files, got %d", n)
	}
}

// TestCommitBoard_RemovalStaged confirms that a deleted (tracked) file is
// staged as a removal and committed.
func TestCommitBoard_RemovalStaged(t *testing.T) {
	root := initRepo(t)
	p := filepath.Join(root, "1-task.md")
	writeFile(t, p, "# task 1")

	// Commit the file so it becomes tracked.
	if err := git.CommitBoard(root, "north: create 1", []string{p}, nil); err != nil {
		t.Fatal(err)
	}

	// Delete the file and pass it as a removal.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := git.CommitBoard(root, "north: delete 1", nil, []string{p}); err != nil {
		t.Fatalf("CommitBoard (removal) returned error: %v", err)
	}
	if n := commitCount(t, root); n != 2 {
		t.Errorf("expected 2 commits after removal, got %d", n)
	}
}

// TestCommitBoard_UntrackedRemoval covers deleting a file git never tracked —
// e.g. a task created before auto_commit was enabled. There is nothing for git
// to record, so CommitBoard must succeed without committing (a bare
// `git add -A -- <gone-path>` would fail with an unmatched pathspec).
func TestCommitBoard_UntrackedRemoval(t *testing.T) {
	root := initRepo(t)
	p := filepath.Join(root, "1-task.md")
	writeFile(t, p, "# task 1")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	if err := git.CommitBoard(root, "north: delete 1", nil, []string{p}); err != nil {
		t.Fatalf("CommitBoard (untracked removal) returned error: %v", err)
	}
	if n := commitCount(t, root); n != 0 {
		t.Errorf("expected 0 commits, got %d", n)
	}
}

// TestCommitBoard_BoardInSubdir confirms that CommitBoard works when the board
// is in a subdirectory (not the repo root).
func TestCommitBoard_BoardInSubdir(t *testing.T) {
	root := initRepo(t)
	boardDir := filepath.Join(root, "north")
	p := filepath.Join(boardDir, "1-task.md")
	writeFile(t, p, "# task 1")

	if err := git.CommitBoard(boardDir, "north: create 1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}
	if n := commitCount(t, root); n != 1 {
		t.Errorf("expected 1 commit, got %d", n)
	}
}

// TestCommitBoard_LinkedWorktree confirms commits land on the linked
// worktree's branch (the previous go-git implementation silently failed here).
func TestCommitBoard_LinkedWorktree(t *testing.T) {
	root := initRepo(t)
	runGit(t, root, "commit", "--allow-empty", "-q", "-m", "init")
	wt := filepath.Join(t.TempDir(), "feature")
	runGit(t, root, "worktree", "add", "-q", wt, "-b", "feature")

	p := filepath.Join(wt, "1-task.md")
	writeFile(t, p, "# task 1")
	if err := git.CommitBoard(wt, "north: create 1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard in worktree returned error: %v", err)
	}
	if msg := runGit(t, wt, "log", "-1", "--format=%s"); msg != "north: create 1" {
		t.Errorf("worktree commit missing; last message %q", msg)
	}
	// Nothing left staged-but-uncommitted.
	if status := runGit(t, wt, "status", "--porcelain"); status != "" {
		t.Errorf("worktree left dirty after commit:\n%s", status)
	}
}

// TestCommitBoard_UnrelatedStagedFileUntouched confirms that files the user
// has staged outside the given paths are neither committed nor unstaged —
// CommitBoard scopes add and commit with an explicit pathspec.
func TestCommitBoard_UnrelatedStagedFileUntouched(t *testing.T) {
	root := initRepo(t)
	unrelated := filepath.Join(root, "unrelated.txt")
	writeFile(t, unrelated, "user work in progress")
	runGit(t, root, "add", "--", unrelated)

	p := filepath.Join(root, "1-task.md")
	writeFile(t, p, "# task 1")
	if err := git.CommitBoard(root, "north: create 1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}

	// Still staged, not committed.
	if status := runGit(t, root, "status", "--porcelain"); !strings.Contains(status, "A  unrelated.txt") {
		t.Errorf("unrelated file no longer staged:\n%s", status)
	}
	if files := runGit(t, root, "log", "-1", "--name-only", "--format="); strings.Contains(files, "unrelated.txt") {
		t.Errorf("unrelated file was committed:\n%s", files)
	}
}

// TestCommitBoard_NoIdentity confirms the fallback identity is used when the
// user has no git user.name/email configured (the previous implementation
// errored after the file write).
func TestCommitBoard_NoIdentity(t *testing.T) {
	isolateGitConfig(t)
	root := t.TempDir()

	// init without setting a local identity (unlike initRepo).
	runGit(t, root, "init", "-q")
	p := filepath.Join(root, "1-task.md")
	writeFile(t, p, "# task 1")
	if err := git.CommitBoard(root, "north: create 1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard without identity returned error: %v", err)
	}
	author := runGit(t, root, "log", "-1", "--format=%an <%ae>")
	if author != "north <north@localhost>" {
		t.Errorf("expected fallback identity, got %q", author)
	}
}

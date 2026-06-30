package git_test

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/SamP-S/north/internal/git"
)

func initRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	root := t.TempDir()
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	return root, repo
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

func commitCount(t *testing.T, repo *gogit.Repository) int {
	t.Helper()
	ref, err := repo.Head()
	if err != nil {
		return 0
	}
	iter, err := repo.Log(&gogit.LogOptions{From: ref.Hash()})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	_ = iter.ForEach(func(_ *object.Commit) error { n++; return nil })
	return n
}

// TestCommitBoard_NotInGitRepo confirms CommitBoard is a silent no-op when
// the board directory is not inside a git work tree.
func TestCommitBoard_NotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	if err := git.CommitBoard(dir, "test", []string{}, nil); err != nil {
		t.Errorf("expected nil for non-repo, got %v", err)
	}
}

// TestCommitBoard_NoPaths confirms that an empty path list produces no commit.
func TestCommitBoard_NoPaths(t *testing.T) {
	root, repo := initRepo(t)
	if err := git.CommitBoard(root, "north: test", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := commitCount(t, repo); n != 0 {
		t.Errorf("expected 0 commits for empty paths, got %d", n)
	}
}

// TestCommitBoard_StagesAndCommits confirms that a new file is staged and
// committed with the supplied message.
func TestCommitBoard_StagesAndCommits(t *testing.T) {
	root, repo := initRepo(t)
	p := filepath.Join(root, "task-1.md")
	writeFile(t, p, "# task 1")

	if err := git.CommitBoard(root, "north: create task-1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}
	if n := commitCount(t, repo); n != 1 {
		t.Errorf("expected 1 commit, got %d", n)
	}
}

// TestCommitBoard_MultipleFiles confirms that several paths are staged in a
// single commit.
func TestCommitBoard_MultipleFiles(t *testing.T) {
	root, repo := initRepo(t)
	paths := []string{
		filepath.Join(root, "task-1.md"),
		filepath.Join(root, "task-2.md"),
	}
	for _, p := range paths {
		writeFile(t, p, "# "+filepath.Base(p))
	}

	if err := git.CommitBoard(root, "north: batch", paths, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}
	if n := commitCount(t, repo); n != 1 {
		t.Errorf("expected 1 commit for multiple files, got %d", n)
	}
}

// TestCommitBoard_RemovalStaged confirms that a deleted (tracked) file is
// staged as a removal and committed.
func TestCommitBoard_RemovalStaged(t *testing.T) {
	root, repo := initRepo(t)
	p := filepath.Join(root, "task-1.md")
	writeFile(t, p, "# task 1")

	// Commit the file so it becomes tracked.
	if err := git.CommitBoard(root, "north: create task-1", []string{p}, nil); err != nil {
		t.Fatal(err)
	}

	// Delete the file and pass it as a removal.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := git.CommitBoard(root, "north: delete task-1", nil, []string{p}); err != nil {
		t.Fatalf("CommitBoard (removal) returned error: %v", err)
	}
	if n := commitCount(t, repo); n != 2 {
		t.Errorf("expected 2 commits after removal, got %d", n)
	}
}

// TestCommitBoard_DetectsDotGit confirms that CommitBoard works when the board
// is in a subdirectory (not the repo root), relying on DetectDotGit.
func TestCommitBoard_DetectsDotGit(t *testing.T) {
	root, repo := initRepo(t)
	boardDir := filepath.Join(root, "north")
	if err := os.MkdirAll(boardDir, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(boardDir, "task-1.md")
	writeFile(t, p, "# task 1")

	if err := git.CommitBoard(boardDir, "north: create task-1", []string{p}, nil); err != nil {
		t.Fatalf("CommitBoard returned error: %v", err)
	}
	if n := commitCount(t, repo); n != 1 {
		t.Errorf("expected 1 commit, got %d", n)
	}
}

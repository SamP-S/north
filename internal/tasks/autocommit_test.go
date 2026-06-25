package tasks_test

import (
	"testing"

	"strings"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/tasks"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func initRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	root := t.TempDir()
	repo, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	boardDir, err := board.InitBoard(root)
	if err != nil {
		t.Fatal(err)
	}
	return boardDir, repo
}

func commitMessages(t *testing.T, repo *gogit.Repository) []string {
	t.Helper()
	ref, err := repo.Head()
	if err != nil {
		return nil // no commits yet
	}
	iter, err := repo.Log(&gogit.LogOptions{From: ref.Hash()})
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	_ = iter.ForEach(func(c *object.Commit) error {
		msgs = append(msgs, c.Message)
		return nil
	})
	return msgs
}

func TestAutoCommitCreatesCommit(t *testing.T) {
	boardDir, repo := initRepo(t)
	if _, err := board.WriteConfig(boardDir, board.Config{AutoCommit: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(boardDir, "committed task", "", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range commitMessages(t, repo) {
		if strings.HasPrefix(m, "north: create task-1") {
			found = true
		}
	}
	if !found {
		t.Errorf("no create commit found: %v", commitMessages(t, repo))
	}
}

func TestNoCommitWhenDisabled(t *testing.T) {
	boardDir, repo := initRepo(t)
	if _, err := tasks.Create(boardDir, "x", "", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Head(); err == nil {
		t.Error("expected no commits, but HEAD is valid")
	}
}

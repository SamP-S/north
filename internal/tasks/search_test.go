package tasks

import (
	"testing"

	"github.com/SamP-S/north/internal/models"
)

func TestMatchesSearch(t *testing.T) {
	task := &models.Task{
		ID:       "42",
		Title:    "Fix Login Flow",
		Assignee: "Agent-A",
		Labels:   []string{"auth", "Backend"},
		Body:     "The SSO redirect loops forever.",
	}
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"empty query matches", "", true},
		{"id", "42", true},
		{"title", "login", true},
		{"title case-insensitive", "LOGIN", true},
		{"assignee", "agent-a", true},
		{"assignee case-insensitive", "AGENT", true},
		{"body", "sso redirect", true},
		{"body case-insensitive", "Forever", true},
		{"label", "auth", true},
		{"label case-insensitive", "backend", true},
		{"no match", "frontend", false},
		{"id substring only", "424", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesSearch(task, tc.query); got != tc.want {
				t.Errorf("MatchesSearch(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// TestMatchesSearchEmptyFields verifies a task with empty optional fields only
// matches on the fields it has.
func TestMatchesSearchEmptyFields(t *testing.T) {
	task := &models.Task{ID: "7", Title: "Bare"}
	if !MatchesSearch(task, "bare") {
		t.Error("title should match")
	}
	if MatchesSearch(task, "anyone") {
		t.Error("empty assignee/labels/body must not match a non-empty query")
	}
}

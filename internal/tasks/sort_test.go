package tasks_test

import (
	"testing"
	"time"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func ts(sec int) *time.Time {
	t := time.Date(2026, 7, 1, 0, 0, sec, 0, time.UTC)
	return &t
}

func TestSortKeys(t *testing.T) {
	mk := func(id, title, assignee string, updated *time.Time) *models.Task {
		return &models.Task{ID: id, Title: title, Assignee: assignee, UpdatedAt: updated}
	}
	base := []*models.Task{
		mk("1", "banana", "zoe", ts(30)),
		mk("2", "Apple", "", ts(10)),
		mk("3", "cherry", "amy", ts(20)),
	}

	cases := []struct {
		key  tasks.SortKey
		desc bool
		want []string
	}{
		{tasks.SortID, true, []string{"3", "2", "1"}},
		{tasks.SortID, false, []string{"1", "2", "3"}},
		{tasks.SortUpdated, true, []string{"1", "3", "2"}},
		{tasks.SortUpdated, false, []string{"2", "3", "1"}},
		{tasks.SortTitle, false, []string{"2", "1", "3"}}, // case-insensitive
		{tasks.SortTitle, true, []string{"3", "1", "2"}},
		// Unassigned ("2") is last in both directions.
		{tasks.SortAssignee, false, []string{"3", "1", "2"}},
		{tasks.SortAssignee, true, []string{"1", "3", "2"}},
	}
	for _, c := range cases {
		got := append([]*models.Task(nil), base...)
		tasks.Sort(got, c.key, c.desc)
		for i, w := range c.want {
			if got[i].ID != w {
				t.Errorf("%s desc=%v pos %d: got %s want %s", c.key, c.desc, i, got[i].ID, w)
			}
		}
	}
}

func TestParseSortKey(t *testing.T) {
	if _, err := tasks.ParseSortKey("updated"); err != nil {
		t.Errorf("updated should parse: %v", err)
	}
	if _, err := tasks.ParseSortKey("priority"); err == nil {
		t.Error("unknown key should be rejected")
	}
}

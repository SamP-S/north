// Package models holds the board data model: a single task object and its
// status.
//
// Statuses are hardcoded for the MVP (configurable statuses are deferred). The
// status of a task is the folder it lives in; the "status" frontmatter key is a
// synced mirror.
package models

import "time"

// TaskStatus is one of the six hardcoded board states.
type TaskStatus string

const (
	Draft      TaskStatus = "draft"
	Ready      TaskStatus = "ready"
	InProgress TaskStatus = "in_progress"
	Done       TaskStatus = "done"
	Failed     TaskStatus = "failed"
	Blocked    TaskStatus = "blocked"
)

// StatusDirs lists the status folders created by `north init`, in board order.
var StatusDirs = []TaskStatus{Draft, Ready, InProgress, Done, Failed, Blocked}

// Transitions is the legal status transition table. draft→ready is the human
// gate; failed/blocked/done return to ready for rework. Illegal jumps are
// rejected (Conflict).
var Transitions = map[TaskStatus]map[TaskStatus]bool{
	Draft:      {Ready: true},
	Ready:      {InProgress: true},
	InProgress: {Done: true, Failed: true, Blocked: true},
	Done:       {Ready: true},
	Failed:     {Ready: true},
	Blocked:    {Ready: true},
}

// IsStatus reports whether s is a known status.
func IsStatus(s TaskStatus) bool {
	_, ok := Transitions[s]
	return ok
}

// Task is one board task. Path is where the file currently lives on disk.
type Task struct {
	ID        string
	Title     string
	Status    TaskStatus
	Path      string
	Agent     string
	Labels    []string
	DependsOn []string
	CreatedAt *time.Time
	UpdatedAt *time.Time
	Body      string
	Archived  bool
}

func isoOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// ToMap returns a stable, JSON-serialisable view used by --json and MCP tools.
func (t *Task) ToMap() map[string]any {
	labels := t.Labels
	if labels == nil {
		labels = []string{}
	}
	deps := t.DependsOn
	if deps == nil {
		deps = []string{}
	}
	return map[string]any{
		"id":         t.ID,
		"title":      t.Title,
		"status":     string(t.Status),
		"agent":      t.Agent,
		"labels":     labels,
		"depends_on": deps,
		"created_at": isoOrNil(t.CreatedAt),
		"updated_at": isoOrNil(t.UpdatedAt),
		"archived":   t.Archived,
		"body":       t.Body,
	}
}

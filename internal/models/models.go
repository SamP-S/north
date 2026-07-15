// Package models holds the board data model: a single task, its lifecycle
// state, and its workflow status.
//
// North uses two orthogonal axes:
//   - State is the lifecycle location — the folder a task lives in
//     (drafts/ tasks/ archive/). It is the source of truth for "where" a task is.
//   - Status is the workflow column (ready/in_progress/done/failed/blocked),
//     stored only in frontmatter. It only changes while the task is active.
//
// Both axes allow free movement within a fixed value set: any status can move
// to any other status, and any state to any other state, in a single step —
// but unknown values are rejected. The status/state *lists* are hardcoded for
// the MVP (configurable statuses are deferred).
package models

import "time"

// TaskState is the lifecycle location of a task — the folder it lives in.
type TaskState string

const (
	// StateDraft is a task in drafts/ — not yet on the active board.
	StateDraft TaskState = "draft"
	// StateActive is a task in tasks/ — visible on the board.
	StateActive TaskState = "active"
	// StateArchive is a task in archive/ — off the board, kept for history.
	StateArchive TaskState = "archive"
)

// StateDirs maps each state to its on-disk folder name (in board order).
var StateDirs = map[TaskState]string{
	StateDraft:   "drafts",
	StateActive:  "tasks",
	StateArchive: "archive",
}

// StateOrder lists the states in board order (drafts → tasks → archive).
var StateOrder = []TaskState{StateDraft, StateActive, StateArchive}

// TaskStatus is the workflow column, stored in frontmatter.
type TaskStatus string

const (
	// Ready is a task waiting to be picked up.
	Ready TaskStatus = "ready"
	// InProgress is a task currently being worked on.
	InProgress TaskStatus = "in_progress"
	// Done is a task completed successfully (terminal).
	Done TaskStatus = "done"
	// Failed is a task abandoned or unsuccessful (terminal).
	Failed TaskStatus = "failed"
	// Blocked is a task parked on something outside it.
	Blocked TaskStatus = "blocked"
)

// Statuses lists every workflow status in board order — a left→right flow:
// blocked sits beside in_progress (a parked task), terminal states last.
var Statuses = []TaskStatus{Ready, InProgress, Blocked, Done, Failed}

// DefaultStatus is the status a new task carries.
const DefaultStatus = Ready

// IsStatus reports whether s is a known status. Status moves are freeform:
// any known status is a legal target from any other (active tasks only).
func IsStatus(s TaskStatus) bool {
	for _, st := range Statuses {
		if st == s {
			return true
		}
	}
	return false
}

// IsState reports whether s is a known state. State moves are freeform: any
// known state is a legal target from any other.
func IsState(s TaskState) bool {
	_, ok := StateDirs[s]
	return ok
}

// StateForDir returns the state whose folder is dir.
func StateForDir(dir string) (TaskState, bool) {
	for s, d := range StateDirs {
		if d == dir {
			return s, true
		}
	}
	return "", false
}

// StateIndex returns the board-order position of a state (for stable sorting).
func StateIndex(s TaskState) int {
	for i, st := range StateOrder {
		if st == s {
			return i
		}
	}
	return len(StateOrder)
}

// Task is one board task. Path is where the file currently lives on disk;
// State is derived from its folder, Status from its frontmatter.
type Task struct {
	ID        string
	Title     string
	State     TaskState
	Status    TaskStatus
	Path      string
	Assignee  string
	Labels    []string
	DependsOn []string
	CreatedAt *time.Time
	UpdatedAt *time.Time
	Body      string
}

func isoOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// ToMap returns a stable, JSON-serialisable view used by --json output.
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
		"state":      string(t.State),
		"status":     string(t.Status),
		"assignee":   t.Assignee,
		"labels":     labels,
		"depends_on": deps,
		"created_at": isoOrNil(t.CreatedAt),
		"updated_at": isoOrNil(t.UpdatedAt),
		"body":       t.Body,
	}
}

// snapshot.go — tolerant whole-board loading.
//
// A Snapshot parses every task file on the board exactly once. Files that fail
// to parse (or carry a duplicate id) become Warnings instead of aborting the
// load, so one bad file can never take down list/board/TUI. All read paths —
// listing, lookups, counts, dependents — derive from a Snapshot; mutations
// still operate per-file.
package tasks

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
)

// Warning describes a task file the snapshot could not fully use.
type Warning struct {
	File string // filename (base) the warning is about
	Err  string // human-readable problem
}

func (w Warning) String() string { return fmt.Sprintf("%s: %s", w.File, w.Err) }

// Snapshot is a one-shot, tolerant parse of the whole board.
type Snapshot struct {
	Tasks    []*models.Task // sorted by state (board order), then id
	Warnings []Warning
	byID     map[string]*models.Task
}

// Load reads every task file on the board. Only filesystem-level failures
// return an error; per-file parse problems and duplicate ids become Warnings.
func Load(boardDir string) (*Snapshot, error) {
	files, err := board.TaskFiles(boardDir)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{byID: make(map[string]*models.Task, len(files))}
	for _, path := range files {
		task, err := loadTask(path)
		if err != nil {
			snap.Warnings = append(snap.Warnings, Warning{File: filepath.Base(path), Err: err.Error()})
			continue
		}
		if prev, ok := snap.byID[task.ID]; ok {
			snap.Warnings = append(snap.Warnings, Warning{
				File: filepath.Base(path),
				Err:  fmt.Sprintf("duplicate id %q (already used by %s); run `north doctor --fix`", task.ID, filepath.Base(prev.Path)),
			})
			continue
		}
		snap.byID[task.ID] = task
		snap.Tasks = append(snap.Tasks, task)
	}
	sort.SliceStable(snap.Tasks, func(i, j int) bool {
		si, sj := models.StateIndex(snap.Tasks[i].State), models.StateIndex(snap.Tasks[j].State)
		if si != sj {
			return si < sj
		}
		return idNum(snap.Tasks[i].ID) < idNum(snap.Tasks[j].ID)
	})
	return snap, nil
}

// Get returns the task with the given id, or nil.
func (s *Snapshot) Get(id string) *models.Task { return s.byID[id] }

// Filter returns tasks in the given states (all when empty), optionally
// narrowed to one status ("" for any). Order follows Snapshot.Tasks.
func (s *Snapshot) Filter(states []models.TaskState, status string) []*models.Task {
	wanted := map[models.TaskState]bool{}
	for _, st := range states {
		wanted[st] = true
	}
	out := make([]*models.Task, 0, len(s.Tasks))
	for _, t := range s.Tasks {
		if len(wanted) > 0 && !wanted[t.State] {
			continue
		}
		if status != "" && string(t.Status) != status {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Dependents returns every task whose DependsOn contains id (exact match).
// Returns an empty non-nil slice if none are found.
func (s *Snapshot) Dependents(id string) []*models.Task {
	out := make([]*models.Task, 0)
	for _, t := range s.Tasks {
		for _, dep := range t.DependsOn {
			if dep == id {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// StatusCounts returns counts of active tasks per status, in board order.
func (s *Snapshot) StatusCounts() []StatusCount {
	counts := map[models.TaskStatus]int{}
	for _, t := range s.Tasks {
		if t.State == models.StateActive {
			counts[t.Status]++
		}
	}
	out := make([]StatusCount, len(models.Statuses))
	for i, st := range models.Statuses {
		out[i] = StatusCount{Status: string(st), Count: counts[st]}
	}
	return out
}

// StateCount reports how many tasks are in a given state.
func (s *Snapshot) StateCount(state models.TaskState) int {
	n := 0
	for _, t := range s.Tasks {
		if t.State == state {
			n++
		}
	}
	return n
}

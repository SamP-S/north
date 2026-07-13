// next.go — work-picking for agents: peek at, or atomically claim, the next
// workable task.
//
// "Workable" means: active, status ready, unassigned, every dependency
// resolved, and carrying every requested label. Order is deterministic
// (lowest id first) so concurrent pickers agree on what "next" means.
//
// Take exists because the two-call claim (`list` then `move in_progress`) has
// a TOCTOU race: the board lock makes each mutation atomic on its own, but
// nothing spans the read-decide-write, and SetStatus treats an unchanged
// status as a silent no-op — so two agents can both believe they claimed the
// same task. Take closes the window by selecting and claiming under a single
// lock hold. Do not "optimise" it back into list+move.
package tasks

import (
	"fmt"
	"strings"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/models"
)

// hasLabels reports whether the task carries every requested label.
func hasLabels(t *models.Task, labels []string) bool {
	if len(labels) == 0 {
		return true
	}
	have := map[string]bool{}
	for _, l := range t.Labels {
		have[l] = true
	}
	for _, l := range labels {
		if !have[l] {
			return false
		}
	}
	return true
}

// workable returns up to limit workable tasks in take order (limit <= 0
// means all). Snapshot order is state then ascending id, so the first entry
// is the lowest-id active+ready+unassigned+deps-met task matching the labels.
func workable(snap *Snapshot, labels []string, limit int) []*models.Task {
	var out []*models.Task
	for _, t := range snap.Tasks {
		if t.State != models.StateActive || t.Status != models.Ready {
			continue
		}
		if t.Assignee != "" || !hasLabels(t, labels) {
			continue
		}
		if len(snap.UnmetDeps(t)) > 0 {
			continue
		}
		out = append(out, t)
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out
}

// Next returns up to limit workable tasks in take order without touching
// anything (a pure read; limit <= 0 means all). An empty result with a nil
// error means nothing is workable — a normal outcome, not a failure. The
// snapshot warnings are returned alongside.
func Next(boardDir string, labels []string, limit int) ([]*models.Task, []Warning, error) {
	snap, err := Load(boardDir)
	if err != nil {
		return nil, nil, err
	}
	return workable(snap, labels, limit), snap.Warnings, nil
}

// claimable refuses (Conflict) unless t could be handed out by take: active,
// ready, unassigned, and — beyond what selection needs — deps met.
func claimable(snap *Snapshot, t *models.Task) error {
	switch {
	case t.State != models.StateActive:
		return errors.Conflict(fmt.Sprintf(
			"task %s is %s — activate it first (`north task state %s active`)", t.ID, t.State, t.ID))
	case t.Status != models.Ready:
		return errors.Conflict(fmt.Sprintf("task %s is %s, not ready", t.ID, t.Status))
	case t.Assignee != "":
		return errors.Conflict(fmt.Sprintf("task %s is already assigned to %q", t.ID, t.Assignee))
	}
	if unmet := snap.UnmetDeps(t); len(unmet) > 0 {
		return errors.Conflict(fmt.Sprintf(
			"task %s has unmet dependencies: %s", t.ID, strings.Join(unmet, ", ")))
	}
	return nil
}

// Take atomically claims a task for assignee: selection/validation,
// status=in_progress, and the assignee are all applied under one board-lock
// hold, so concurrent Takes hand out different tasks. With taskID empty the
// next workable task is selected — a nil task with a nil error means nothing
// is workable. With taskID set, that specific task is claimed instead:
// unknown ids are NotFound, and a task take could not have selected (not
// active, not ready, already assigned, unmet deps) is refused with a
// Conflict naming the reason — no steal, no overrides. When the board's
// max_wip is set (> 0) and assignee already holds that many active
// in_progress tasks, Take refuses with a conflict. Only deps-met tasks are
// ever claimed, so no additional dependency enforcement applies.
func Take(boardDir, assignee, taskID string, labels []string) (*models.Task, []Warning, error) {
	if strings.TrimSpace(assignee) == "" {
		return nil, nil, errors.Invalid("take needs an assignee (pass --assignee or set NORTH_AGENT)")
	}
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return nil, nil, err
	}
	unlock, err := board.Lock(boardDir)
	if err != nil {
		return nil, nil, err
	}
	defer unlock()
	snap, err := Load(boardDir)
	if err != nil {
		return nil, nil, err
	}
	if cfg.MaxWIP > 0 {
		var held []string
		for _, t := range snap.Tasks {
			// Assignees match case-insensitively: "Claude-A" and "claude-a"
			// are one agent.
			if t.State == models.StateActive && t.Status == models.InProgress && strings.EqualFold(t.Assignee, assignee) {
				held = append(held, t.ID)
			}
		}
		if len(held) >= cfg.MaxWIP {
			return nil, nil, errors.Conflict(fmt.Sprintf(
				"%q already has %d task(s) in progress (max_wip %d): %s — finish or move them first",
				assignee, len(held), cfg.MaxWIP, strings.Join(held, ", ")))
		}
	}
	var task *models.Task
	if taskID != "" {
		task = snap.Get(taskID)
		if task == nil {
			return nil, nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
		}
		if err := claimable(snap, task); err != nil {
			return nil, nil, err
		}
	} else {
		picked := workable(snap, labels, 1)
		if len(picked) == 0 {
			return nil, snap.Warnings, nil
		}
		task = picked[0]
	}
	task.Status = models.InProgress
	task.Assignee = assignee
	n := now()
	task.UpdatedAt = &n
	saved, err := save(boardDir, task, task.Path, fmt.Sprintf("north: take %s (%s)", task.ID, assignee))
	if err != nil {
		return nil, nil, err
	}
	return saved, snap.Warnings, nil
}

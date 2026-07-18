// Package tasks holds task operations over the board: create, read, list, edit,
// status changes, state changes (draft/active/archive), delete.
//
// Every task is one Markdown file. North uses two orthogonal axes: the task's
// state is the folder it lives in (drafts/ tasks/ archive/), and its status is
// a frontmatter key (ready/in_progress/blocked/done/failed). Movement is free
// within each fixed value set — any value to any other in one step, in any
// state. Mutations serialise through the advisory board lock and optionally
// make a local git commit when auto_commit is set.
package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/errors"
	"github.com/SamP-S/north/internal/git"
	"github.com/SamP-S/north/internal/models"
	"gopkg.in/yaml.v3"
)

func now() time.Time { return time.Now().UTC() }

// ParseStatus coerces a string to a TaskStatus, raising Invalid on unknown values.
func ParseStatus(value string) (models.TaskStatus, error) {
	s := models.TaskStatus(value)
	if !models.IsStatus(s) {
		allowed := make([]string, len(models.Statuses))
		for i, d := range models.Statuses {
			allowed[i] = string(d)
		}
		return "", errors.Invalid(fmt.Sprintf("unknown status %q (expected one of: %s)", value, strings.Join(allowed, ", ")))
	}
	return s, nil
}

// ParseState coerces a string to a TaskState, raising Invalid on unknown values.
func ParseState(value string) (models.TaskState, error) {
	s := models.TaskState(value)
	if !models.IsState(s) {
		allowed := make([]string, len(models.StateOrder))
		for i, d := range models.StateOrder {
			allowed[i] = string(d)
		}
		return "", errors.Invalid(fmt.Sprintf("unknown state %q (expected one of: %s)", value, strings.Join(allowed, ", ")))
	}
	return s, nil
}

// --- load / persist ----------------------------------------------------------------

func parseDT(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.999999999Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// taskIDRe restricts frontmatter ids to bare numbers: the id is interpolated
// into filenames, so anything else could escape the board directory.
var taskIDRe = regexp.MustCompile(`^[0-9]+$`)

// loadTask parses one task file. Every error names the file.
func loadTask(path string) (*models.Task, error) {
	base := filepath.Base(path)
	fail := func(msg string) error {
		return errors.Invalid(fmt.Sprintf("failed to parse %s: %s", base, msg))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fail(err.Error())
	}
	meta, body, err := splitFrontmatter(normalizeNewlines(string(data)))
	if err != nil {
		return nil, fail(err.Error())
	}
	_, fm, err := parseFront(meta)
	if err != nil {
		return nil, fail(err.Error())
	}
	// State comes from the folder; status always comes from frontmatter.
	dirName := filepath.Base(filepath.Dir(path))
	state, ok := models.StateForDir(dirName)
	if !ok {
		return nil, fail(fmt.Sprintf("unknown state folder %q", dirName))
	}
	status, err := ParseStatus(fm.Status)
	if err != nil {
		return nil, fail(err.Error())
	}
	if fm.ID == "" || fm.Title == "" {
		return nil, fail("missing id/title")
	}
	if !taskIDRe.MatchString(fm.ID) {
		return nil, fail(fmt.Sprintf("id %q is not a bare number", fm.ID))
	}
	return &models.Task{
		ID:        fm.ID,
		Title:     fm.Title,
		State:     state,
		Status:    status,
		Path:      path,
		Assignee:  fm.Assignee,
		Labels:    fm.Labels,
		DependsOn: fm.DependsOn,
		CreatedAt: parseDT(fm.CreatedAt),
		UpdatedAt: parseDT(fm.UpdatedAt),
		Body:      strings.Trim(body, "\n"),
	}, nil
}

// loadPrevFront best-effort re-reads a task file's frontmatter document so
// unknown keys survive the rewrite. Returns nil when unavailable.
func loadPrevFront(path string) *yaml.Node {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	meta, _, err := splitFrontmatter(normalizeNewlines(string(data)))
	if err != nil {
		return nil
	}
	doc, _, err := parseFront(meta)
	if err != nil {
		return nil
	}
	return doc
}

func targetPath(boardDir string, task *models.Task) string {
	return filepath.Join(board.StateDir(boardDir, task.State), board.TaskFilename(task.ID, task.Title))
}

// save writes the task to its target path (atomically: temp file + rename),
// removes the old file on a move/rename, and auto-commits when configured.
// extraPaths are committed alongside the task file (e.g. the config.yml
// last_id bump an allocation just wrote).
func save(boardDir string, task *models.Task, oldPath, message string, extraPaths ...string) (*models.Task, error) {
	target := targetPath(boardDir, task)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	content, err := renderTask(task, loadPrevFront(oldPath))
	if err != nil {
		return nil, err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	var removed []string
	if oldPath != "" && !samePath(oldPath, target) {
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Remove(oldPath); err != nil {
				return nil, err
			}
			removed = append(removed, oldPath)
		}
	}
	task.Path = target
	if err := commit(boardDir, append([]string{target}, extraPaths...), removed, message); err != nil {
		return nil, err
	}
	return task, nil
}

func commit(boardDir string, paths, removed []string, message string) error {
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return err
	}
	if cfg.AutoCommit {
		return git.CommitBoard(boardDir, message, paths, removed)
	}
	return nil
}

// find returns one task by id via a tolerant snapshot load, so unrelated
// malformed files never block the lookup.
func find(boardDir, taskID string) (*models.Task, error) {
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	if t := snap.Get(taskID); t != nil {
		return t, nil
	}
	return nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
}

// --- public operations -----------------------------------------------------------

// depsPolicy loads the board's deps_enforcement level.
func depsPolicy(boardDir string) (board.DepsEnforcement, error) {
	cfg, err := board.LoadConfig(boardDir)
	if err != nil {
		return "", err
	}
	return cfg.DepsEnforcement, nil
}

// dedupIDs drops duplicates from a depends_on list, preserving order.
func dedupIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// checkDeps vets a proposed depends_on set for the task with taskID ("" on
// create, when no cycle or self-reference is possible yet): dangling ids,
// self-references, and cycles. At hint level every problem is a warning; at
// validated and strict each is refused.
func checkDeps(snap *Snapshot, level board.DepsEnforcement, taskID string, deps []string) ([]string, error) {
	hint := level == board.DepsHint
	var warns []string
	for _, id := range deps {
		switch {
		case taskID != "" && id == taskID:
			msg := fmt.Sprintf("depends_on: task %s cannot depend on itself", taskID)
			if !hint {
				return nil, errors.Invalid(msg)
			}
			warns = append(warns, msg)
		case snap.Get(id) == nil:
			if !hint {
				return nil, errors.Invalid(fmt.Sprintf("depends_on: task %q not found", id))
			}
			warns = append(warns, fmt.Sprintf(
				"depends_on: task %q does not exist (yet) — this will bind to whatever gets that id", id))
		}
	}
	if taskID != "" {
		graph := map[string][]string{}
		for _, t := range snap.Tasks {
			graph[t.ID] = t.DependsOn
		}
		graph[taskID] = deps
		if cycle := findCycle(graph); len(cycle) > 0 {
			msg := fmt.Sprintf("depends_on: dependency cycle: %s", strings.Join(cycle, " → "))
			if !hint {
				return nil, errors.Invalid(msg)
			}
			warns = append(warns, msg)
		}
	}
	return warns, nil
}

// checkStatusDeps vets a status move against the task's unmet dependencies:
// finishing (done) or starting (in_progress) a task whose dependencies are
// unresolved warns at hint/validated and is refused at strict.
func checkStatusDeps(snap *Snapshot, level board.DepsEnforcement, task *models.Task, target models.TaskStatus) ([]string, error) {
	if target != models.Done && target != models.InProgress {
		return nil, nil
	}
	unmet := snap.UnmetDeps(task)
	if len(unmet) == 0 {
		return nil, nil
	}
	msg := fmt.Sprintf("task %s has unmet dependencies: %s", task.ID, strings.Join(unmet, ", "))
	if level == board.DepsStrict {
		return nil, errors.Conflict(msg + " — complete them, or edit --depends-on")
	}
	return []string{msg}, nil
}

// Dependents returns every task whose DependsOn slice contains taskID (exact
// match), scanning all state folders. Returns an empty non-nil slice if none
// are found.
func Dependents(boardDir, taskID string) ([]*models.Task, error) {
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	return snap.Dependents(taskID), nil
}

// TemplateBody returns the board's task-template.md content, used to fill
// bodyless creates. A missing or empty template means a blank body — the
// template is an init-time scaffold, not a runtime fallback.
func TemplateBody(boardDir string) string {
	data, err := os.ReadFile(filepath.Join(boardDir, board.TemplateName))
	if err != nil {
		return ""
	}
	return strings.Trim(normalizeNewlines(string(data)), "\n")
}

// Create makes a task in drafts/ with status ready. The returned warnings
// are advisory dependency notes (hint level only — the op still succeeded).
func Create(boardDir, title, assignee string, labels, dependsOn []string, body string) (*models.Task, []string, error) {
	if strings.TrimSpace(title) == "" {
		return nil, nil, errors.Invalid("task title must not be empty")
	}
	level, err := depsPolicy(boardDir)
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
	dependsOn = dedupIDs(dependsOn)
	warns, err := checkDeps(snap, level, "", dependsOn)
	if err != nil {
		return nil, nil, err
	}
	id, err := board.AllocateID(boardDir)
	if err != nil {
		return nil, nil, err
	}
	n := now()
	task := &models.Task{
		ID:        id,
		Title:     strings.TrimSpace(title),
		State:     models.StateDraft,
		Status:    models.DefaultStatus,
		Assignee:  assignee,
		Labels:    labels,
		DependsOn: dependsOn,
		CreatedAt: &n,
		UpdatedAt: &n,
		Body:      body,
	}
	saved, err := save(boardDir, task, "", fmt.Sprintf("north: create %s", task.ID),
		filepath.Join(boardDir, board.ConfigName))
	if err != nil {
		return nil, nil, err
	}
	return saved, warns, nil
}

// Get returns one task by id (searches all state folders).
func Get(boardDir, taskID string) (*models.Task, error) {
	return find(boardDir, taskID)
}

// EditOpts carries optional field changes for Edit. Nil fields are unchanged.
type EditOpts struct {
	Title      *string
	Assignee   *string
	Labels     *[]string
	DependsOn  *[]string
	Body       *string
	AppendBody *string // appended to the body with a blank-line separator
}

// Edit changes a task's fields/body. UpdatedAt is bumped. The returned
// warnings are advisory dependency notes (hint level only).
func Edit(boardDir, taskID string, opts EditOpts) (*models.Task, []string, error) {
	level, err := depsPolicy(boardDir)
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
	task := snap.Get(taskID)
	if task == nil {
		return nil, nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
	}
	oldPath := task.Path
	if opts.Title != nil {
		if strings.TrimSpace(*opts.Title) == "" {
			return nil, nil, errors.Invalid("task title must not be empty")
		}
		task.Title = strings.TrimSpace(*opts.Title)
	}
	if opts.Assignee != nil {
		task.Assignee = *opts.Assignee
	}
	if opts.Labels != nil {
		task.Labels = *opts.Labels
	}
	var warns []string
	if opts.DependsOn != nil {
		deps := dedupIDs(*opts.DependsOn)
		warns, err = checkDeps(snap, level, taskID, deps)
		if err != nil {
			return nil, nil, err
		}
		task.DependsOn = deps
	}
	if opts.Body != nil {
		task.Body = *opts.Body
	}
	if opts.AppendBody != nil && strings.TrimSpace(*opts.AppendBody) != "" {
		if strings.TrimSpace(task.Body) == "" {
			task.Body = *opts.AppendBody
		} else {
			task.Body = strings.Trim(task.Body, "\n") + "\n\n" + *opts.AppendBody
		}
	}
	n := now()
	task.UpdatedAt = &n
	saved, err := save(boardDir, task, oldPath, fmt.Sprintf("north: edit %s", task.ID))
	if err != nil {
		return nil, nil, err
	}
	return saved, warns, nil
}

// SetStatus changes a task's workflow status (frontmatter only; the file
// stays in its state folder). Movement is free within the status set —
// though status is only visible on the board while the task is active, and
// finishing/starting with unmet dependencies warns (hint/validated) or is
// refused (strict). The returned warnings are advisory (op succeeded).
func SetStatus(boardDir, taskID string, newStatus string) (*models.Task, []string, error) {
	target, err := ParseStatus(newStatus)
	if err != nil {
		return nil, nil, err
	}
	level, err := depsPolicy(boardDir)
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
	task := snap.Get(taskID)
	if task == nil {
		return nil, nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
	}
	if target == task.Status {
		// Even a no-op surfaces the assignee note — a crash-recovery reset
		// to ready must never silently starve the task.
		return task, readyAssigneeWarning(task, target), nil
	}
	warns, err := checkStatusDeps(snap, level, task, target)
	if err != nil {
		return nil, nil, err
	}
	warns = append(warns, readyAssigneeWarning(task, target)...)
	task.Status = target
	n := now()
	task.UpdatedAt = &n
	saved, err := save(boardDir, task, task.Path, fmt.Sprintf("north: %s → %s", task.ID, target))
	if err != nil {
		return nil, nil, err
	}
	return saved, warns, nil
}

// readyAssigneeWarning warns when a task is set to ready while still assigned:
// next/take only offer unassigned work, so the task would sit invisible.
func readyAssigneeWarning(task *models.Task, target models.TaskStatus) []string {
	if target != models.Ready || task.Assignee == "" {
		return nil
	}
	return []string{fmt.Sprintf(
		"task %s is still assigned to %q; next/take will not offer it (clear with `north task edit %s --assignee \"\"`)",
		task.ID, task.Assignee, task.ID)}
}

// SetState moves a task's file between state folders (draft/active/archive),
// preserving status. Freeform: any valid state is reachable from any other.
func SetState(boardDir, taskID string, newState string) (*models.Task, error) {
	target, err := ParseState(newState)
	if err != nil {
		return nil, err
	}
	unlock, err := board.Lock(boardDir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.State == target {
		return task, nil
	}
	oldPath := task.Path
	task.State = target
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, oldPath, fmt.Sprintf("north: %s state → %s", task.ID, target))
}

// Cleanup archives active done tasks (optionally only those older than N
// days; olderThanDays <= 0 means all). The lock is held for the whole run —
// snapshot and every archive write — so a task moved off done by a
// concurrent process mid-run can never be archived stale. With dryRun the
// candidates are returned without locking or writing anything.
func Cleanup(boardDir string, olderThanDays int, dryRun bool) ([]*models.Task, error) {
	var cutoff *time.Time
	if olderThanDays > 0 {
		c := now().Add(-time.Duration(olderThanDays) * 24 * time.Hour)
		cutoff = &c
	}
	if !dryRun {
		unlock, err := board.Lock(boardDir)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	var archived []*models.Task
	for _, task := range snap.Filter([]models.TaskState{models.StateActive}, string(models.Done)) {
		if cutoff != nil && (task.UpdatedAt == nil || task.UpdatedAt.After(*cutoff)) {
			continue
		}
		if dryRun {
			archived = append(archived, task)
			continue
		}
		// Save directly — we already hold the lock, so SetState would deadlock.
		oldPath := task.Path
		task.State = models.StateArchive
		n := now()
		task.UpdatedAt = &n
		a, err := save(boardDir, task, oldPath, fmt.Sprintf("north: %s state → archive", task.ID))
		if err != nil {
			return nil, err
		}
		archived = append(archived, a)
	}
	return archived, nil
}

// Delete removes a task file. At validated/strict the dependents are healed
// (the deleted id is dropped from their depends_on, under the same lock
// hold); at hint the dangling references stay, warned. Returned warnings
// describe either outcome.
func Delete(boardDir, taskID string) ([]string, error) {
	level, err := depsPolicy(boardDir)
	if err != nil {
		return nil, err
	}
	unlock, err := board.Lock(boardDir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	task := snap.Get(taskID)
	if task == nil {
		return nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
	}
	dependents := snap.Dependents(taskID)
	if err := os.Remove(task.Path); err != nil {
		return nil, err
	}
	if err := commit(boardDir, nil, []string{task.Path}, fmt.Sprintf("north: delete %s", task.ID)); err != nil {
		return nil, err
	}
	if len(dependents) == 0 {
		return nil, nil
	}
	ids := make([]string, len(dependents))
	for i, d := range dependents {
		ids[i] = d.ID
	}
	if level == board.DepsHint {
		return []string{fmt.Sprintf(
			"%s depended on %s — their depends_on now dangles", strings.Join(ids, ", "), taskID)}, nil
	}
	// Heal: rewrite each dependent without the deleted id (save directly —
	// we already hold the lock, so Edit would deadlock).
	for _, d := range dependents {
		kept := d.DependsOn[:0]
		for _, id := range d.DependsOn {
			if id != taskID {
				kept = append(kept, id)
			}
		}
		d.DependsOn = kept
		n := now()
		d.UpdatedAt = &n
		if _, err := save(boardDir, d, d.Path, fmt.Sprintf("north: heal deps of %s after delete %s", d.ID, taskID)); err != nil {
			return nil, fmt.Errorf("healing depends_on of %s: %w", d.ID, err)
		}
	}
	return []string{fmt.Sprintf("removed %s from depends_on of %s", taskID, strings.Join(ids, ", "))}, nil
}

// StatusCount is one row of the board summary.
type StatusCount struct {
	Status string
	Count  int
}

func idNum(taskID string) int {
	n, err := strconv.Atoi(taskID)
	if err != nil {
		return 0
	}
	return n
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return aa == bb
}

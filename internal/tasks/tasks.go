// Package tasks holds task operations over the board: create, read, list, edit,
// status changes, state changes (draft/active/archive), delete.
//
// Every task is one Markdown file. North uses two orthogonal axes: the task's
// state is the folder it lives in (drafts/ tasks/ archive/), and its status is a
// frontmatter key (ready/in_progress/done/failed/blocked) that only changes
// while the task is active. Both axes are freeform — any value can move to any
// other in one step. Each mutation optionally makes a local git commit when
// auto_commit is set.
package tasks

import (
	"fmt"
	"os"
	"path/filepath"
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
	return &models.Task{
		ID:        fm.ID,
		Title:     fm.Title,
		State:     state,
		Status:    status,
		Path:      path,
		Agent:     fm.Agent,
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
func save(boardDir string, task *models.Task, oldPath, message string) (*models.Task, error) {
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
	if err := commit(boardDir, []string{target}, removed, message); err != nil {
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

// validateDeps checks that every task ID in ids exists somewhere on the board.
func validateDeps(snap *Snapshot, ids []string) error {
	for _, id := range ids {
		if snap.Get(id) == nil {
			return errors.Invalid(fmt.Sprintf("depends_on: task %q not found", id))
		}
	}
	return nil
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

// Create makes a task in drafts/ with status ready.
func Create(boardDir, title, agent string, labels, dependsOn []string, body string) (*models.Task, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.Invalid("task title must not be empty")
	}
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	if err := validateDeps(snap, dependsOn); err != nil {
		return nil, err
	}
	id, err := board.NextID(boardDir)
	if err != nil {
		return nil, err
	}
	n := now()
	task := &models.Task{
		ID:        id,
		Title:     strings.TrimSpace(title),
		State:     models.StateDraft,
		Status:    models.DefaultStatus,
		Agent:     agent,
		Labels:    labels,
		DependsOn: dependsOn,
		CreatedAt: &n,
		UpdatedAt: &n,
		Body:      body,
	}
	return save(boardDir, task, "", fmt.Sprintf("north: create %s", task.ID))
}

// Get returns one task by id (searches all state folders).
func Get(boardDir, taskID string) (*models.Task, error) {
	return find(boardDir, taskID)
}

// EditOpts carries optional field changes for Edit. Nil fields are unchanged.
type EditOpts struct {
	Title      *string
	Agent      *string
	Labels     *[]string
	DependsOn  *[]string
	Body       *string
	AppendBody *string // appended to the body with a blank-line separator
}

// Edit changes a task's fields/body. UpdatedAt is bumped.
func Edit(boardDir, taskID string, opts EditOpts) (*models.Task, error) {
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	task := snap.Get(taskID)
	if task == nil {
		return nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
	}
	oldPath := task.Path
	if opts.Title != nil {
		if strings.TrimSpace(*opts.Title) == "" {
			return nil, errors.Invalid("task title must not be empty")
		}
		task.Title = strings.TrimSpace(*opts.Title)
	}
	if opts.Agent != nil {
		task.Agent = *opts.Agent
	}
	if opts.Labels != nil {
		task.Labels = *opts.Labels
	}
	if opts.DependsOn != nil {
		if err := validateDeps(snap, *opts.DependsOn); err != nil {
			return nil, err
		}
		task.DependsOn = *opts.DependsOn
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
	return save(boardDir, task, oldPath, fmt.Sprintf("north: edit %s", task.ID))
}

// SetStatus changes an active task's workflow status (frontmatter only; the
// file stays in tasks/). Freeform: any valid status is reachable from any other.
func SetStatus(boardDir, taskID string, newStatus string) (*models.Task, error) {
	target, err := ParseStatus(newStatus)
	if err != nil {
		return nil, err
	}
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.State != models.StateActive {
		return nil, errors.Conflict(fmt.Sprintf(
			"task %q is %s; move it to the active state before changing status", taskID, task.State))
	}
	if target == task.Status {
		return task, nil
	}
	task.Status = target
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, task.Path, fmt.Sprintf("north: %s → %s", task.ID, target))
}

// SetState moves a task's file between state folders (draft/active/archive),
// preserving status. Freeform: any valid state is reachable from any other.
func SetState(boardDir, taskID string, newState string) (*models.Task, error) {
	target, err := ParseState(newState)
	if err != nil {
		return nil, err
	}
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

// Cleanup archives active done tasks (optionally only those older than N days).
// olderThanDays <= 0 means archive all active done tasks.
func Cleanup(boardDir string, olderThanDays int) ([]*models.Task, error) {
	var cutoff *time.Time
	if olderThanDays > 0 {
		c := now().Add(-time.Duration(olderThanDays) * 24 * time.Hour)
		cutoff = &c
	}
	snap, err := Load(boardDir)
	if err != nil {
		return nil, err
	}
	done := snap.Filter([]models.TaskState{models.StateActive}, string(models.Done))
	var archived []*models.Task
	for _, task := range done {
		if cutoff != nil && (task.UpdatedAt == nil || task.UpdatedAt.After(*cutoff)) {
			continue
		}
		a, err := SetState(boardDir, task.ID, string(models.StateArchive))
		if err != nil {
			return nil, err
		}
		archived = append(archived, a)
	}
	return archived, nil
}

// Delete removes a task file.
func Delete(boardDir, taskID string) error {
	task, err := find(boardDir, taskID)
	if err != nil {
		return err
	}
	if err := os.Remove(task.Path); err != nil {
		return err
	}
	return commit(boardDir, nil, []string{task.Path}, fmt.Sprintf("north: delete %s", task.ID))
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

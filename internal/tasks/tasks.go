// Package tasks holds task operations over the board: create, read, list, edit,
// status changes, lifecycle (promote/demote/archive/restore), delete.
//
// Every task is one Markdown file. North uses two orthogonal axes: the task's
// state is the folder it lives in (drafts/ tasks/ archive/), and its status is a
// frontmatter key (ready/in_progress/done/failed/blocked) that only changes
// while the task is active. Each mutation optionally makes a local git commit
// when auto_commit is set.
package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// --- frontmatter -----------------------------------------------------------------

// frontMeta mirrors the task frontmatter, in field order.
type frontMeta struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Status    string   `yaml:"status"`
	Agent     string   `yaml:"agent"`
	Labels    []string `yaml:"labels"`
	DependsOn []string `yaml:"depends_on"`
	CreatedAt *string  `yaml:"created_at"`
	UpdatedAt *string  `yaml:"updated_at"`
}

func parseDT(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.999999999Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t
		}
	}
	return nil
}

func loadTask(path string) (*models.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Invalid(fmt.Sprintf("failed to parse %s: %v", filepath.Base(path), err))
	}
	meta, body, err := splitFrontmatter(string(data))
	if err != nil {
		return nil, errors.Invalid(fmt.Sprintf("failed to parse %s: %v", filepath.Base(path), err))
	}
	var fm frontMeta
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return nil, errors.Invalid(fmt.Sprintf("failed to parse %s: %v", filepath.Base(path), err))
	}
	// State comes from the folder; status always comes from frontmatter.
	dirName := filepath.Base(filepath.Dir(path))
	state, ok := models.StateForDir(dirName)
	if !ok {
		return nil, errors.Invalid(fmt.Sprintf("failed to parse %s: unknown state folder %q", filepath.Base(path), dirName))
	}
	status, err := ParseStatus(fm.Status)
	if err != nil {
		return nil, err
	}
	if fm.ID == "" || fm.Title == "" {
		return nil, errors.Invalid(fmt.Sprintf("failed to parse %s: missing id/title", filepath.Base(path)))
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
		Body:      strings.TrimSpace(body),
	}, nil
}

// splitFrontmatter separates a leading "---\n...\n---" YAML block from the body.
func splitFrontmatter(content string) (meta, body string, err error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---\n") && content != "---" {
		// No frontmatter: whole thing is body.
		return "", content, nil
	}
	rest := strings.TrimPrefix(content, "---\n")
	if rest == content {
		rest = strings.TrimPrefix(content, "---")
	}
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("unterminated frontmatter block")
	}
	meta = rest[:idx]
	body = rest[idx+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	return meta, body, nil
}

func isoPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func render(task *models.Task) (string, error) {
	labels := task.Labels
	if labels == nil {
		labels = []string{}
	}
	deps := task.DependsOn
	if deps == nil {
		deps = []string{}
	}
	fm := frontMeta{
		ID:        task.ID,
		Title:     task.Title,
		Status:    string(task.Status),
		Agent:     task.Agent,
		Labels:    labels,
		DependsOn: deps,
		CreatedAt: isoPtr(task.CreatedAt),
		UpdatedAt: isoPtr(task.UpdatedAt),
	}
	out, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	front := strings.TrimRight(string(out), "\n")
	body := strings.TrimSpace(task.Body)
	if body != "" {
		return fmt.Sprintf("---\n%s\n---\n\n%s\n", front, body), nil
	}
	return fmt.Sprintf("---\n%s\n---\n", front), nil
}

// --- persist ---------------------------------------------------------------------

func targetPath(boardDir string, task *models.Task) string {
	return filepath.Join(board.StateDir(boardDir, task.State), board.TaskFilename(task.ID, task.Title))
}

func save(boardDir string, task *models.Task, oldPath, message string) (*models.Task, error) {
	target := targetPath(boardDir, task)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	content, err := render(task)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
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

func find(boardDir, taskID string) (*models.Task, error) {
	files, err := board.TaskFiles(boardDir)
	if err != nil {
		return nil, err
	}
	prefix := taskID + " - "
	for _, path := range files {
		if strings.HasPrefix(filepath.Base(path), prefix) {
			task, err := loadTask(path)
			if err != nil {
				return nil, err
			}
			if task.ID == taskID {
				return task, nil
			}
		}
	}
	return nil, errors.NotFound(fmt.Sprintf("task %q not found", taskID))
}

// --- public operations -----------------------------------------------------------

// Create makes a task in drafts/ with status ready.
func Create(boardDir, title, agent string, labels, dependsOn []string, body string) (*models.Task, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.Invalid("task title must not be empty")
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

// List returns tasks in the given states (all states if none given), optionally
// filtered by status ("" for any). Results are ordered by state then id.
func List(boardDir string, states []models.TaskState, status string) ([]*models.Task, error) {
	var wanted *models.TaskStatus
	if status != "" {
		s, err := ParseStatus(status)
		if err != nil {
			return nil, err
		}
		wanted = &s
	}
	files, err := board.TaskFiles(boardDir, states...)
	if err != nil {
		return nil, err
	}
	var out []*models.Task
	for _, path := range files {
		task, err := loadTask(path)
		if err != nil {
			return nil, err
		}
		if wanted != nil && task.Status != *wanted {
			continue
		}
		out = append(out, task)
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := models.StateIndex(out[i].State), models.StateIndex(out[j].State)
		if si != sj {
			return si < sj
		}
		return idNum(out[i].ID) < idNum(out[j].ID)
	})
	return out, nil
}

// Edit changes a task's fields/body. UpdatedAt is bumped. Pass nil for a field
// to leave it unchanged. Status and state are not edited here.
func Edit(boardDir, taskID string, title, agent *string, labels, dependsOn *[]string, body *string) (*models.Task, error) {
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	oldPath := task.Path
	if title != nil {
		if strings.TrimSpace(*title) == "" {
			return nil, errors.Invalid("task title must not be empty")
		}
		task.Title = strings.TrimSpace(*title)
	}
	if agent != nil {
		task.Agent = *agent
	}
	if labels != nil {
		task.Labels = *labels
	}
	if dependsOn != nil {
		task.DependsOn = *dependsOn
	}
	if body != nil {
		task.Body = *body
	}
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, oldPath, fmt.Sprintf("north: edit %s", task.ID))
}

// SetStatus changes an active task's workflow status (frontmatter only; the file
// stays in tasks/). Rejected unless the task is active and the transition legal.
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
			"task %q is %s; promote it to active before changing status", taskID, task.State))
	}
	if target == task.Status {
		return task, nil
	}
	if !models.Transitions[task.Status][target] {
		return nil, errors.Conflict(fmt.Sprintf(
			"illegal transition %s → %s (from %s you can go to: %s)",
			task.Status, target, task.Status, allowedStatuses(task.Status)))
	}
	task.Status = target
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, task.Path, fmt.Sprintf("north: %s → %s", task.ID, target))
}

// Promote moves a draft onto the active board (drafts/ → tasks/).
func Promote(boardDir, taskID string) (*models.Task, error) {
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.State != models.StateDraft {
		return nil, errors.Conflict(fmt.Sprintf("only draft tasks can be promoted (task %q is %s)", taskID, task.State))
	}
	return changeState(boardDir, task, models.StateActive, "promote")
}

// Demote sends an active task back to drafts/ (tasks/ → drafts/).
func Demote(boardDir, taskID string) (*models.Task, error) {
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.State != models.StateActive {
		return nil, errors.Conflict(fmt.Sprintf("only active tasks can be demoted (task %q is %s)", taskID, task.State))
	}
	return changeState(boardDir, task, models.StateDraft, "demote")
}

// Archive moves a task into archive/ (off the active board), preserving status.
func Archive(boardDir, taskID string) (*models.Task, error) {
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.State == models.StateArchive {
		return nil, errors.Conflict(fmt.Sprintf("task %q is already archived", taskID))
	}
	return changeState(boardDir, task, models.StateArchive, "archive")
}

// Restore brings an archived task back to drafts (archive/ → drafts/), giving
// the human a chance to review it before promoting to active.
func Restore(boardDir, taskID string) (*models.Task, error) {
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.State != models.StateArchive {
		return nil, errors.Conflict(fmt.Sprintf("only archived tasks can be restored (task %q is %s)", taskID, task.State))
	}
	return changeState(boardDir, task, models.StateDraft, "restore")
}

// changeState moves a task's file between state folders, preserving status.
func changeState(boardDir string, task *models.Task, target models.TaskState, verb string) (*models.Task, error) {
	if !models.StateTransitions[task.State][target] {
		return nil, errors.Conflict(fmt.Sprintf("cannot %s task %q from %s", verb, task.ID, task.State))
	}
	oldPath := task.Path
	task.State = target
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, oldPath, fmt.Sprintf("north: %s %s", verb, task.ID))
}

// Cleanup archives active done tasks (optionally only those older than N days).
// olderThanDays <= 0 means archive all active done tasks.
func Cleanup(boardDir string, olderThanDays int) ([]*models.Task, error) {
	var cutoff *time.Time
	if olderThanDays > 0 {
		c := now().Add(-time.Duration(olderThanDays) * 24 * time.Hour)
		cutoff = &c
	}
	done, err := List(boardDir, []models.TaskState{models.StateActive}, string(models.Done))
	if err != nil {
		return nil, err
	}
	var archived []*models.Task
	for _, task := range done {
		if cutoff != nil && (task.UpdatedAt == nil || task.UpdatedAt.After(*cutoff)) {
			continue
		}
		a, err := Archive(boardDir, task.ID)
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

// StatusCounts returns counts of active tasks per status, in board order.
func StatusCounts(boardDir string) ([]StatusCount, error) {
	tasks, err := List(boardDir, []models.TaskState{models.StateActive}, "")
	if err != nil {
		return nil, err
	}
	counts := map[models.TaskStatus]int{}
	for _, t := range tasks {
		counts[t.Status]++
	}
	out := make([]StatusCount, len(models.Statuses))
	for i, s := range models.Statuses {
		out[i] = StatusCount{Status: string(s), Count: counts[s]}
	}
	return out, nil
}

// StatusCount is one row of the board summary.
type StatusCount struct {
	Status string
	Count  int
}

// StateCount reports how many tasks are in a given state.
func StateCount(boardDir string, state models.TaskState) (int, error) {
	ts, err := List(boardDir, []models.TaskState{state}, "")
	if err != nil {
		return 0, err
	}
	return len(ts), nil
}

func allowedStatuses(from models.TaskStatus) string {
	var ss []string
	for s := range models.Transitions[from] {
		ss = append(ss, string(s))
	}
	if len(ss) == 0 {
		return "(none)"
	}
	sort.Strings(ss)
	return strings.Join(ss, ", ")
}

func idNum(taskID string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(taskID, "task-"))
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

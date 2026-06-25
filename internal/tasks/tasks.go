// Package tasks holds task operations over the board: create, read, list, edit,
// move, archive, delete.
//
// Every task is one Markdown file. Its status is the folder it lives in; the
// "status" frontmatter key is kept in sync. Each mutation optionally makes a
// local git commit when auto_commit is set.
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
		allowed := make([]string, len(models.StatusDirs))
		for i, d := range models.StatusDirs {
			allowed[i] = string(d)
		}
		return "", errors.Invalid(fmt.Sprintf("unknown status %q (expected one of: %s)", value, strings.Join(allowed, ", ")))
	}
	return s, nil
}

// --- frontmatter -----------------------------------------------------------------

// frontMeta mirrors the task frontmatter, in board order.
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
	archived := filepath.Base(filepath.Dir(path)) == board.ArchiveDir
	var status models.TaskStatus
	if archived {
		status, err = ParseStatus(fm.Status)
	} else {
		status, err = ParseStatus(filepath.Base(filepath.Dir(path)))
	}
	if err != nil {
		return nil, err
	}
	if fm.ID == "" || fm.Title == "" {
		return nil, errors.Invalid(fmt.Sprintf("failed to parse %s: missing id/title", filepath.Base(path)))
	}
	return &models.Task{
		ID:        fm.ID,
		Title:     fm.Title,
		Status:    status,
		Path:      path,
		Agent:     fm.Agent,
		Labels:    fm.Labels,
		DependsOn: fm.DependsOn,
		CreatedAt: parseDT(fm.CreatedAt),
		UpdatedAt: parseDT(fm.UpdatedAt),
		Body:      strings.TrimSpace(body),
		Archived:  archived,
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
	folder := string(task.Status)
	if task.Archived {
		folder = board.ArchiveDir
	}
	return filepath.Join(boardDir, folder, board.TaskFilename(task.ID, task.Title))
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
	files, err := board.TaskFiles(boardDir, true)
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

// Create makes a task in draft/.
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
		Status:    models.Draft,
		Agent:     agent,
		Labels:    labels,
		DependsOn: dependsOn,
		CreatedAt: &n,
		UpdatedAt: &n,
		Body:      body,
	}
	return save(boardDir, task, "", fmt.Sprintf("north: create %s", task.ID))
}

// Get returns one task by id (searches all folders incl. archive).
func Get(boardDir, taskID string) (*models.Task, error) {
	return find(boardDir, taskID)
}

// List returns active tasks (add archived ones with archived=true). status may
// be "" for no filter.
func List(boardDir, status string, archived bool) ([]*models.Task, error) {
	var wanted *models.TaskStatus
	if status != "" {
		s, err := ParseStatus(status)
		if err != nil {
			return nil, err
		}
		wanted = &s
	}
	files, err := board.TaskFiles(boardDir, archived)
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
		if out[i].Archived != out[j].Archived {
			return !out[i].Archived
		}
		return idNum(out[i].ID) < idNum(out[j].ID)
	})
	return out, nil
}

// Edit changes a task's fields/body. UpdatedAt is bumped. Pass nil for a field
// to leave it unchanged.
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

// Move changes a task's status (validates the transition; moves the file).
func Move(boardDir, taskID string, newStatus string) (*models.Task, error) {
	target, err := ParseStatus(newStatus)
	if err != nil {
		return nil, err
	}
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.Archived {
		return nil, errors.Conflict(fmt.Sprintf("task %q is archived; cannot change its status", taskID))
	}
	if target == task.Status {
		return task, nil
	}
	if !models.Transitions[task.Status][target] {
		allowed := allowedTransitions(task.Status)
		return nil, errors.Conflict(fmt.Sprintf(
			"illegal transition %s → %s (from %s you can go to: %s)",
			task.Status, target, task.Status, allowed))
	}
	oldPath := task.Path
	task.Status = target
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, oldPath, fmt.Sprintf("north: %s → %s", task.ID, target))
}

// Archive moves a task into archive/ (off the active board).
func Archive(boardDir, taskID string) (*models.Task, error) {
	task, err := find(boardDir, taskID)
	if err != nil {
		return nil, err
	}
	if task.Archived {
		return nil, errors.Conflict(fmt.Sprintf("task %q is already archived", taskID))
	}
	oldPath := task.Path
	task.Archived = true
	n := now()
	task.UpdatedAt = &n
	return save(boardDir, task, oldPath, fmt.Sprintf("north: archive %s", task.ID))
}

// Cleanup archives all done/ tasks (optionally only those older than N days).
// olderThanDays <= 0 means archive all done tasks.
func Cleanup(boardDir string, olderThanDays int) ([]*models.Task, error) {
	var cutoff *time.Time
	if olderThanDays > 0 {
		c := now().Add(-time.Duration(olderThanDays) * 24 * time.Hour)
		cutoff = &c
	}
	done, err := List(boardDir, string(models.Done), false)
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
	tasks, err := List(boardDir, "", false)
	if err != nil {
		return nil, err
	}
	counts := map[models.TaskStatus]int{}
	for _, t := range tasks {
		counts[t.Status]++
	}
	out := make([]StatusCount, len(models.StatusDirs))
	for i, s := range models.StatusDirs {
		out[i] = StatusCount{Status: string(s), Count: counts[s]}
	}
	return out, nil
}

// StatusCount is one row of the board summary.
type StatusCount struct {
	Status string
	Count  int
}

func allowedTransitions(from models.TaskStatus) string {
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

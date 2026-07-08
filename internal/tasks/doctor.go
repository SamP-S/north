// doctor.go — whole-board integrity checking and repair.
//
// Doctor scans every task file and reports problems the normal per-command
// paths only warn about (or cannot see at all): unparseable files, duplicate
// ids (e.g. after a git merge of two branches that each created the same id),
// filename↔frontmatter id drift, dangling depends_on references, dependency
// cycles, and CRLF line endings. With fix enabled it repairs what is safe to
// repair mechanically.
package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/SamP-S/north/internal/board"
)

// Issue is one problem found by Doctor.
type Issue struct {
	Kind   string // unparseable | duplicate-id | id-drift | dangling-dep | cycle | crlf | gitattributes
	File   string // filename (base) the issue is about ("" for board-wide issues)
	Detail string
	Fixed  bool // true when fix mode repaired it
}

func (i Issue) String() string {
	state := ""
	if i.Fixed {
		state = " (fixed)"
	}
	if i.File == "" {
		return fmt.Sprintf("%s: %s%s", i.Kind, i.Detail, state)
	}
	return fmt.Sprintf("%s: %s: %s%s", i.Kind, i.File, i.Detail, state)
}

// Doctor checks the whole board and returns every issue found, most severe
// first. With fix true it also repairs: CRLF files are rewritten with LF,
// duplicate ids are renumbered to fresh ids (the first holder keeps the id, so
// existing depends_on references stay valid), drifted filenames are renamed to
// match their frontmatter id, dangling depends_on references are removed, and
// a missing north/.gitattributes is restored. Unparseable files and cycles
// are report-only.
func Doctor(boardDir string, fix bool) ([]Issue, error) {
	var issues []Issue
	if fix {
		unlock, err := board.Lock(boardDir)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}

	// Pass 0: board-file scaffolding init owns.
	if _, err := os.Stat(filepath.Join(boardDir, board.GitattributesName)); os.IsNotExist(err) {
		issue := Issue{Kind: "gitattributes", File: board.GitattributesName,
			Detail: "missing (guards board files against CRLF drift on clone)"}
		if fix {
			if err := board.WriteGitattributes(boardDir); err == nil {
				issue.Fixed = true
			}
		}
		issues = append(issues, issue)
	}

	// Pass 1: raw line-ending check (before parsing, since CRLF used to break it).
	files, err := board.TaskFiles(boardDir)
	if err != nil {
		return nil, err
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue // surfaces as unparseable below
		}
		if strings.Contains(string(raw), "\r") {
			issue := Issue{Kind: "crlf", File: filepath.Base(path), Detail: "CRLF line endings"}
			if fix {
				if err := os.WriteFile(path, []byte(normalizeNewlines(string(raw))), 0o644); err == nil {
					issue.Fixed = true
				}
			}
			issues = append(issues, issue)
		}
	}

	// Pass 2: parse everything, group by id.
	files, err = board.TaskFiles(boardDir)
	if err != nil {
		return nil, err
	}
	byID := map[string][]string{} // id -> files (in listing order)
	idSet := map[string]bool{}
	type parsed struct {
		path string
		id   string
		deps []string
	}
	var tasksParsed []parsed
	highest := 0
	for _, path := range files {
		task, err := loadTask(path)
		if err != nil {
			issues = append(issues, Issue{Kind: "unparseable", File: filepath.Base(path), Detail: err.Error()})
			continue
		}
		byID[task.ID] = append(byID[task.ID], path)
		idSet[task.ID] = true
		tasksParsed = append(tasksParsed, parsed{path: path, id: task.ID, deps: task.DependsOn})
		if n, err := strconv.Atoi(task.ID); err == nil && n > highest {
			highest = n
		}
		// Filename ↔ frontmatter id drift.
		m := filenameID(filepath.Base(path))
		if m != "" && m != task.ID {
			issue := Issue{Kind: "id-drift", File: filepath.Base(path),
				Detail: fmt.Sprintf("filename id %q != frontmatter id %q", m, task.ID)}
			if fix {
				target := filepath.Join(filepath.Dir(path), board.TaskFilename(task.ID, task.Title))
				if err := os.Rename(path, target); err == nil {
					issue.Fixed = true
				}
			}
			issues = append(issues, issue)
		}
	}

	// Duplicate ids: first file keeps the id, later ones are renumbered.
	dupIDs := make([]string, 0)
	for id, paths := range byID {
		if len(paths) > 1 {
			dupIDs = append(dupIDs, id)
		}
	}
	sort.Strings(dupIDs)
	for _, id := range dupIDs {
		paths := byID[id]
		for _, path := range paths[1:] {
			issue := Issue{Kind: "duplicate-id", File: filepath.Base(path),
				Detail: fmt.Sprintf("id %q already used by %s", id, filepath.Base(paths[0]))}
			if fix {
				if task, err := loadTask(path); err == nil {
					highest++
					task.ID = strconv.Itoa(highest)
					if _, err := save(boardDir, task, path, fmt.Sprintf("north: doctor renumber %s → %s", id, task.ID)); err == nil {
						issue.Fixed = true
						idSet[task.ID] = true
					}
				}
			}
			issues = append(issues, issue)
		}
	}

	// Dangling depends_on. With fix, the unknown ids are removed from the
	// file's depends_on in one rewrite. (Deliberate forward references on
	// hint-level boards are removed too — running --fix is an explicit ask.)
	for _, p := range tasksParsed {
		var dangling []string
		for _, dep := range p.deps {
			if !idSet[dep] {
				dangling = append(dangling, dep)
			}
		}
		if len(dangling) == 0 {
			continue
		}
		fixed := false
		if fix {
			if task, err := loadTask(p.path); err == nil {
				kept := task.DependsOn[:0]
				for _, dep := range task.DependsOn {
					if idSet[dep] {
						kept = append(kept, dep)
					}
				}
				task.DependsOn = kept
				if _, err := save(boardDir, task, p.path, fmt.Sprintf("north: doctor remove dangling deps of %s", task.ID)); err == nil {
					fixed = true
				}
			}
		}
		for _, dep := range dangling {
			issues = append(issues, Issue{Kind: "dangling-dep", File: filepath.Base(p.path),
				Detail: fmt.Sprintf("depends_on references unknown task %q", dep), Fixed: fixed})
		}
	}

	// Dependency cycles (DFS over the parsed graph).
	deps := map[string][]string{}
	for _, p := range tasksParsed {
		deps[p.id] = p.deps
	}
	if cycle := findCycle(deps); len(cycle) > 0 {
		issues = append(issues, Issue{Kind: "cycle",
			Detail: fmt.Sprintf("dependency cycle: %s", strings.Join(cycle, " → "))})
	}

	return issues, nil
}

// filenameID extracts the leading numeric id from a task filename ("" if none).
func filenameID(base string) string {
	i := strings.IndexByte(base, '-')
	if i <= 0 {
		return ""
	}
	if _, err := strconv.Atoi(base[:i]); err != nil {
		return ""
	}
	return base[:i]
}

// findCycle returns one dependency cycle as an id path (nil when acyclic).
func findCycle(deps map[string][]string) []string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycle []string

	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		for _, dep := range deps[id] {
			switch color[dep] {
			case grey:
				// Found: slice the stack from dep's position.
				for i, s := range stack {
					if s == dep {
						cycle = append(append([]string{}, stack[i:]...), dep)
						return true
					}
				}
			case white:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	ids := make([]string, 0, len(deps))
	for id := range deps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white {
			stack = stack[:0]
			if visit(id) {
				return cycle
			}
		}
	}
	return nil
}

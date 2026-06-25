package render_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/render"
)

func sample() *models.Task {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	return &models.Task{
		ID:        "task-1",
		Title:     "Add login",
		State:     models.StateActive,
		Status:    models.Ready,
		Agent:     "opus4.8",
		Labels:    []string{"auth"},
		DependsOn: []string{"task-4"},
		CreatedAt: &now,
		UpdatedAt: &now,
		Body:      "do the thing",
	}
}

func TestTaskListPlain(t *testing.T) {
	out, err := render.TaskList([]*models.Task{sample()}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "task-1\tactive\tready\tAdd login" {
		t.Errorf("plain output: %q", out)
	}
}

func TestTaskListJSONOmitsBody(t *testing.T) {
	out, err := render.TaskList([]*models.Task{sample()}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 task, got %d", len(got))
	}
	if _, ok := got[0]["body"]; ok {
		t.Error("list JSON should omit body")
	}
	if got[0]["id"] != "task-1" {
		t.Errorf("id: %v", got[0]["id"])
	}
}

func TestTaskDetailJSONIncludesBody(t *testing.T) {
	out, err := render.TaskDetail(sample(), false, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["body"] != "do the thing" {
		t.Errorf("body missing: %v", got["body"])
	}
}

func TestTaskDetailHumanShowsBody(t *testing.T) {
	out, err := render.TaskDetail(sample(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--- body ---") || !strings.Contains(out, "do the thing") {
		t.Errorf("human detail: %q", out)
	}
}

func TestTaskListHuman(t *testing.T) {
	out, err := render.TaskList([]*models.Task{sample()}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// Human output shows id, state, status, and title.
	for _, want := range []string{"task-1", "active", "ready", "Add login"} {
		if !strings.Contains(out, want) {
			t.Errorf("human list missing %q: %q", want, out)
		}
	}
}

func TestTaskListEmpty(t *testing.T) {
	out, err := render.TaskList(nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "(no tasks)" {
		t.Errorf("empty list: %q", out)
	}
}

func TestTaskDetailNoBody(t *testing.T) {
	task := sample()
	task.Body = ""
	out, err := render.TaskDetail(task, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "--- body ---") {
		t.Errorf("should omit body separator when body empty: %q", out)
	}
	if !strings.Contains(out, "state:      active") {
		t.Errorf("detail should show state: %q", out)
	}
}

func TestTaskDetailPlainBody(t *testing.T) {
	out, err := render.TaskDetail(sample(), true, false)
	if err != nil {
		t.Fatal(err)
	}
	// --plain shows the body without the decorative separator.
	if strings.Contains(out, "--- body ---") || !strings.Contains(out, "do the thing") {
		t.Errorf("plain detail: %q", out)
	}
}

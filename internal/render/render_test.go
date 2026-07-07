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
		ID:        "1",
		Title:     "Add login",
		State:     models.StateActive,
		Status:    models.Ready,
		Assignee:  "opus4.8",
		Labels:    []string{"auth"},
		DependsOn: []string{"4"},
		CreatedAt: &now,
		UpdatedAt: &now,
		Body:      "do the thing",
	}
}

func TestTaskListPlain(t *testing.T) {
	out, err := render.TaskList([]*models.Task{sample()}, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "1\tactive\tready\topus4.8\tauth\tAdd login" {
		t.Errorf("plain output: %q", out)
	}
}

func TestTaskListJSONOmitsBody(t *testing.T) {
	out, err := render.TaskList([]*models.Task{sample()}, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Tasks    []map[string]any `json:"tasks"`
		Warnings []string         `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("want 1 task, got %d", len(got.Tasks))
	}
	if _, ok := got.Tasks[0]["body"]; ok {
		t.Error("list JSON should omit body")
	}
	if got.Tasks[0]["id"] != "1" {
		t.Errorf("id: %v", got.Tasks[0]["id"])
	}
	if got.Warnings == nil {
		t.Error("warnings should be [] not null")
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
	out, err := render.TaskList([]*models.Task{sample()}, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// Human output shows id, state, status, and title.
	for _, want := range []string{"1", "active", "ready", "Add login"} {
		if !strings.Contains(out, want) {
			t.Errorf("human list missing %q: %q", want, out)
		}
	}
}

func TestTaskListEmpty(t *testing.T) {
	out, err := render.TaskList(nil, nil, false, false)
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

package tui

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SamP-S/north/internal/board"
	"github.com/SamP-S/north/internal/models"
	"github.com/SamP-S/north/internal/tasks"
)

func newTestBoard(t *testing.T) string {
	t.Helper()
	dir, err := board.InitBoard(t.TempDir())
	if err != nil {
		t.Fatalf("init board: %v", err)
	}
	return dir
}

func mustActive(t *testing.T, dir, title string) *models.Task {
	t.Helper()
	task, _, err := tasks.Create(dir, title, "", nil, nil, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	task, err = tasks.SetState(dir, task.ID, "active")
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	return task
}

// keyRune builds a rune key message.
func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// rootWithTask returns a root model showing one active task on the board.
func rootWithTask(t *testing.T, dir string, task *models.Task) Model {
	t.Helper()
	m := NewModel(dir, Options{})
	m.width, m.height = 80, 24
	m.board.columns = []boardColumn{
		{status: string(task.Status), tasks: []*models.Task{task}, cursor: 0},
	}
	return m
}

func TestDefaultSortNewestFirst(t *testing.T) {
	mk := func(id string) *models.Task { return &models.Task{ID: id} }
	ts := []*models.Task{mk("1"), mk("10"), mk("3"), mk("2")}
	tasks.Sort(ts, tasks.SortID, true)
	want := []string{"10", "3", "2", "1"}
	for i, w := range want {
		if ts[i].ID != w {
			t.Errorf("pos %d: got %q, want %q", i, ts[i].ID, w)
		}
	}
}

// TestSortPickerApplies verifies 'o' opens the sort picker and a selection
// re-orders both views.
func TestSortPickerApplies(t *testing.T) {
	m := NewModel(t.TempDir(), Options{})
	m.width, m.height = 80, 24
	a := &models.Task{ID: "1", Title: "zzz", Status: models.Ready, State: models.StateActive}
	b := &models.Task{ID: "2", Title: "aaa", Status: models.Ready, State: models.StateActive}
	m.board.all = []boardColumn{{status: "ready", tasks: []*models.Task{a, b}}}
	m.board.rebuild()
	m.list.allTasks = []*models.Task{a, b}
	m.list.refilter()

	// Default: id descending — 2 first.
	if m.board.columns[0].tasks[0].ID != "2" || m.list.filtered[0].ID != "2" {
		t.Fatal("default order should be id descending")
	}

	updated, _ := m.Update(keyRune('o'))
	um := updated.(Model)
	if um.modal.mode != modalSortPicker {
		t.Fatalf("o should open the sort picker, got %v", um.modal.mode)
	}
	// Move to "title ↑" and confirm.
	um.modal.cursor = sortIndex(tasks.SortTitle, false)
	updated, cmd := um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um = updated.(Model)
	if cmd == nil {
		t.Fatal("enter should produce the sort message")
	}
	updated, _ = um.Update(cmd())
	um = updated.(Model)
	if got := um.list.filtered[0].Title; got != "aaa" {
		t.Errorf("list should sort by title ascending, got %q first", got)
	}
	if got := um.board.columns[0].tasks[0].Title; got != "aaa" {
		t.Errorf("board should sort by title ascending, got %q first", got)
	}
}

func TestCreateTemplateParses(t *testing.T) {
	tmpl := createTemplate(t.TempDir())
	if tmpl == "" {
		t.Fatal("createTemplate returned empty string")
	}
	doc, err := tasks.ParseEditorDoc(tmpl)
	if err != nil {
		t.Fatalf("template should parse in the task-file format: %v", err)
	}
	if doc.Title != "" {
		t.Errorf("blank template should have an empty title, got %q", doc.Title)
	}
}

func TestEditTemplateRoundTrip(t *testing.T) {
	task := &models.Task{
		Title:     "Fix login",
		Assignee:  "opus",
		Labels:    []string{"auth"},
		DependsOn: []string{"4"},
		Body:      "line one\n\nline two",
	}
	tmpl, err := editTemplate(task)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := tasks.ParseEditorDoc(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != task.Title || doc.Assignee != task.Assignee || doc.Body != task.Body {
		t.Errorf("round trip mismatch: %+v", doc)
	}
	if len(doc.Labels) != 1 || len(doc.DependsOn) != 1 || doc.DependsOn[0] != "4" {
		t.Errorf("lists lost: %+v", doc)
	}
}

// TestDeleteConfirmNamesDependents verifies that pressing 'd' when a dependent
// task exists produces a confirm prompt naming both tasks — in both views.
func TestDeleteConfirmNamesDependents(t *testing.T) {
	dir := newTestBoard(t)
	t1 := mustActive(t, dir, "First task")
	if _, _, err := tasks.Create(dir, "Second task", "", nil, []string{t1.ID}, ""); err != nil {
		t.Fatalf("create dependent: %v", err)
	}

	for _, view := range []viewMode{viewBoard, viewList} {
		m := rootWithTask(t, dir, t1)
		m.view = view
		m.list.filtered = []*models.Task{t1}

		updated, _ := m.Update(keyRune('d'))
		um := updated.(Model)
		if um.modal.mode != modalConfirmDelete {
			t.Fatalf("view %v: expected delete confirm, got %v", view, um.modal.mode)
		}
		if !strings.Contains(um.modal.confirm, "1") || !strings.Contains(um.modal.confirm, "2") {
			t.Errorf("view %v: confirm %q should name target and dependent", view, um.modal.confirm)
		}
	}
}

// TestStatusPickerOpensInBothViews verifies that 'm' opens the shared status
// picker regardless of the active view.
func TestStatusPickerOpensInBothViews(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Active task")

	for _, view := range []viewMode{viewBoard, viewList} {
		m := rootWithTask(t, dir, active)
		m.view = view
		m.list.filtered = []*models.Task{active}

		updated, _ := m.Update(keyRune('m'))
		um := updated.(Model)
		if um.modal.mode != modalStatusPicker {
			t.Fatalf("view %v: expected status picker, got %v", view, um.modal.mode)
		}
	}
}

// TestStatusPickerOnDraftWarns verifies 'm' works for a non-active task
// (status is freeform in any state) and that applying it yields a warning
// notice rather than a plain success.
func TestStatusPickerOnDraftWarns(t *testing.T) {
	dir := newTestBoard(t)
	draft, _, err := tasks.Create(dir, "Draft task", "", nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	m := rootWithTask(t, dir, draft)
	updated, _ := m.Update(keyRune('m'))
	um := updated.(Model)
	if um.modal.mode != modalStatusPicker {
		t.Fatalf("m should open the status picker for a draft, got %v", um.modal.mode)
	}

	// Pick a different status and confirm.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyDown})
	um = updated.(Model)
	updated, cmd := um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um = updated.(Model)
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}
	msg, ok := cmd().(actionDoneMsg)
	if !ok {
		t.Fatalf("expected actionDoneMsg, got %v", msg)
	}
	if msg.notice.level != noticeWarn {
		t.Errorf("status change on a draft should warn, got level %v (%q)",
			msg.notice.level, msg.notice.text)
	}
	task, err := tasks.Get(dir, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status == models.Ready || task.State != models.StateDraft {
		t.Errorf("expected changed status on an unchanged draft, got %s/%s", task.Status, task.State)
	}
}

// TestStatePickerOpensAndApplies verifies that 's' opens the state picker and
// Enter applies the selected state.
func TestStatePickerOpensAndApplies(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Active task")

	m := rootWithTask(t, dir, active)
	updated, _ := m.Update(keyRune('s'))
	um := updated.(Model)
	if um.modal.mode != modalStatePicker {
		t.Fatalf("expected state picker, got %v", um.modal.mode)
	}
	if um.modal.cursor != stateIndex(models.StateActive) {
		t.Errorf("picker cursor should start on the current state")
	}

	// Move to archive and confirm.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyDown})
	um = updated.(Model)
	updated, cmd := um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um = updated.(Model)
	if um.modal.mode != modalNone {
		t.Fatal("enter should close the picker")
	}
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}
	msg, ok := cmd().(actionDoneMsg)
	if !ok || msg.notice.level != noticeSuccess {
		t.Fatalf("state change should succeed with a notice, got %v", msg)
	}
	task, err := tasks.Get(dir, active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != models.StateArchive {
		t.Errorf("expected archive, got %s", task.State)
	}
}

// isQuit reports whether cmd resolves to tea.QuitMsg.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestQuitIsNoOpOnOpenModal verifies that pressing 'q' while a modal is open
// neither quits nor closes it — esc is the only cancel key.
func TestQuitIsNoOpOnOpenModal(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Active task")

	m := rootWithTask(t, dir, active)
	updated, _ := m.Update(keyRune('m'))
	um := updated.(Model)

	updated, cmd := um.Update(keyRune('q'))
	um = updated.(Model)
	if um.modal.mode != modalStatusPicker {
		t.Errorf("expected modal to stay open after q, got %v", um.modal.mode)
	}
	if isQuit(cmd) {
		t.Error("q should not quit the app while a modal is open")
	}

	// esc still cancels.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).modal.mode != modalNone {
		t.Error("expected esc to close the modal")
	}
}

// TestQuitInertInHelp verifies q does nothing while the help overlay is open
// (esc closes it), and quits from the main view.
func TestQuitInertInHelp(t *testing.T) {
	m := NewModel(t.TempDir(), Options{})
	m.width, m.height = 80, 24

	updated, _ := m.Update(keyRune('?'))
	um := updated.(Model)
	if !um.showHelp {
		t.Fatal("? should open help")
	}
	updated, cmd := um.Update(keyRune('q'))
	um = updated.(Model)
	if !um.showHelp {
		t.Error("q should be inert while help is open")
	}
	if isQuit(cmd) {
		t.Error("q should not quit while help is open")
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	um = updated.(Model)
	if um.showHelp {
		t.Error("esc should close the help overlay")
	}
	_, cmd = um.Update(keyRune('q'))
	if !isQuit(cmd) {
		t.Error("q should quit once help is closed")
	}
}

// TestRefreshKeyReloads verifies 'r' emits a reload.
func TestRefreshKeyReloads(t *testing.T) {
	m := NewModel(t.TempDir(), Options{})
	m.width, m.height = 80, 24
	_, cmd := m.Update(keyRune('r'))
	if cmd == nil {
		t.Fatal("r should produce a command")
	}
	if msg := cmd(); msg != (reloadMsg{}) {
		t.Errorf("r should emit reloadMsg, got %v", msg)
	}
}

// TestTruncateMultibyte verifies board card titles are truncated by display
// width, never mid-rune.
func TestTruncateMultibyte(t *testing.T) {
	dir := newTestBoard(t)
	m := newBoardModel(dir)
	m.setSize(40, 20)
	m.columns = []boardColumn{{
		status: "ready",
		tasks: []*models.Task{
			{ID: "1", Title: strings.Repeat("é", 60), Status: models.Ready, State: models.StateActive},
			{ID: "2", Title: strings.Repeat("漢", 40), Status: models.Ready, State: models.StateActive},
		},
	}}
	out := m.renderColumn(0, m.columns[0], 20, 10)
	if strings.Contains(out, "�") {
		t.Error("truncation produced a replacement character (split rune)")
	}
	for _, line := range strings.Split(out, "\n") {
		// No rendered line may exceed the column width.
		if w := len([]rune(line)); w > 0 && strings.Contains(line, "…") && w > 60 {
			t.Errorf("line too wide after truncation: %q", line)
		}
	}
}

// TestSelectByIDClearsFilter verifies Enter-from-board still selects a task
// that the active filter would hide (the root clears the filter and retries).
func TestSelectByIDClearsFilter(t *testing.T) {
	m := NewModel(t.TempDir(), Options{})
	m.width, m.height = 80, 24
	m.list.allTasks = []*models.Task{
		{ID: "1", Title: "alpha", Status: models.Ready, State: models.StateActive},
		{ID: "2", Title: "beta", Status: models.Ready, State: models.StateActive},
	}
	m.searchInput.SetValue("alpha")
	m.setQuery("alpha")

	updated, _ := m.Update(selectTaskMsg{taskID: "2"}) // hidden by the filter
	um := updated.(Model)
	if sel := um.list.selected(); sel == nil || sel.ID != "2" {
		t.Errorf("selectTaskMsg should clear the filter and select 2, got %v", sel)
	}
	if um.query != "" {
		t.Error("filter should be cleared")
	}
}

// TestGlobalFilterOnBoard verifies typing a query with '/' filters the board
// columns in place and esc clears it.
func TestGlobalFilterOnBoard(t *testing.T) {
	m := NewModel(t.TempDir(), Options{})
	m.width, m.height = 80, 24
	m.board.all = []boardColumn{{
		status: "ready",
		tasks: []*models.Task{
			{ID: "1", Title: "alpha", Status: models.Ready, State: models.StateActive},
			{ID: "2", Title: "beta", Status: models.Ready, State: models.StateActive},
		},
	}}
	m.board.rebuild()

	updated, _ := m.Update(keyRune('/'))
	um := updated.(Model)
	if !um.searching {
		t.Fatal("/ should enter search mode")
	}
	for _, r := range "beta" {
		updated, _ = um.Update(keyRune(r))
		um = updated.(Model)
	}
	if got := um.board.columns[0].tasks; len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("filter should narrow the column to task 2, got %v", got)
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um = updated.(Model)
	if um.searching {
		t.Error("enter should leave search mode (filter kept)")
	}
	if um.query != "beta" {
		t.Errorf("query should persist after enter, got %q", um.query)
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	um = updated.(Model)
	if um.query != "" || len(um.board.columns[0].tasks) != 2 {
		t.Error("esc should clear the filter and restore the board")
	}
}

// TestBoardColumnScrolls verifies the selected card is always within the
// rendered window when a column holds more tasks than fit.
func TestBoardColumnScrolls(t *testing.T) {
	dir := newTestBoard(t)
	m := newBoardModel(dir)
	m.setSize(40, 12)
	var ts []*models.Task
	for i := 1; i <= 30; i++ {
		ts = append(ts, &models.Task{
			ID: strconv.Itoa(i), Title: "task-" + strconv.Itoa(i),
			Status: models.Ready, State: models.StateActive,
		})
	}
	m.columns = []boardColumn{{status: "ready", tasks: ts, cursor: 29}}
	out := m.renderColumn(0, m.columns[0], 30, 8)
	if !strings.Contains(out, "task-30") {
		t.Error("cursor at the bottom must be visible in the rendered column")
	}
	if !strings.Contains(out, "↑") {
		t.Error("scrolled column should show the scrolled-up indicator")
	}
	if !strings.Contains(out, "► 30") {
		t.Error("selected card in the focused column should carry the ► cursor")
	}
}

// TestDeleteConfirmAcceptsEnter verifies enter confirms a delete like y.
func TestDeleteConfirmAcceptsEnter(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Doomed task")
	m := rootWithTask(t, dir, active)

	updated, _ := m.Update(keyRune('d'))
	um := updated.(Model)
	if um.modal.mode != modalConfirmDelete {
		t.Fatalf("expected delete confirm, got %v", um.modal.mode)
	}
	updated, cmd := um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um = updated.(Model)
	if um.modal.mode != modalNone {
		t.Fatal("enter should close the confirm")
	}
	if cmd == nil {
		t.Fatal("enter should produce the delete command")
	}
	if msg, ok := cmd().(actionDoneMsg); !ok || msg.notice.level != noticeSuccess {
		t.Fatalf("delete should succeed, got %v", msg)
	}
	if _, err := tasks.Get(dir, active.ID); err == nil {
		t.Error("task should be gone after enter-confirmed delete")
	}
}

// TestTopBottomKeys verifies g/G jump the list cursor.
func TestTopBottomKeys(t *testing.T) {
	dir := newTestBoard(t)
	m := newListModel(dir)
	m.setSize(80, 24)
	for i := 1; i <= 5; i++ {
		m.allTasks = append(m.allTasks, &models.Task{
			ID: strconv.Itoa(i), Title: "t", Status: models.Ready, State: models.StateActive,
		})
	}
	m.refilter()

	m, _ = m.updateListPane(keyRune('G'))
	if m.cursor != 4 {
		t.Errorf("G should jump to bottom, cursor=%d", m.cursor)
	}
	m, _ = m.updateListPane(keyRune('g'))
	if m.cursor != 0 {
		t.Errorf("g should jump to top, cursor=%d", m.cursor)
	}
}

// TestLoadDataSevenColumns verifies the board loads draft | statuses | archive
// columns with the right ordering and sorting.
func TestLoadDataSevenColumns(t *testing.T) {
	dir := newTestBoard(t)
	if _, _, err := tasks.Create(dir, "a draft", "", nil, nil, ""); err != nil { // 1
		t.Fatal(err)
	}
	mustActive(t, dir, "an active") // 2
	arch1 := mustActive(t, dir, "old archive")
	arch2 := mustActive(t, dir, "new archive")
	for _, id := range []string{arch1.ID, arch2.ID} {
		if _, err := tasks.SetState(dir, id, "archive"); err != nil {
			t.Fatal(err)
		}
	}

	data, err := loadData(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.cols) != len(models.Statuses)+2 {
		t.Fatalf("expected %d columns, got %d", len(models.Statuses)+2, len(data.cols))
	}
	first, last := data.cols[0], data.cols[len(data.cols)-1]
	if first.state != models.StateDraft || len(first.tasks) != 1 {
		t.Errorf("first column should be drafts with 1 task, got %q (%d)", first.title, len(first.tasks))
	}
	if last.state != models.StateArchive || len(last.tasks) != 2 {
		t.Errorf("last column should be archive with 2 tasks, got %q (%d)", last.title, len(last.tasks))
	}
	if last.tasks[0].ID != arch1.ID {
		t.Errorf("archive should be ascending by id, got %s first", last.tasks[0].ID)
	}
	if data.cols[1].status != "ready" || data.cols[1].tasks[0].Title != "an active" {
		t.Errorf("second column should be ready actives, got %q", data.cols[1].title)
	}
}

// TestBoardEnterOpensTaskPopup verifies enter on a board card opens the
// read-only popup and esc closes it.
func TestBoardEnterOpensTaskPopup(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Popup task")
	m := rootWithTask(t, dir, active)
	m.list.allTasks = []*models.Task{active}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um := updated.(Model)
	if um.modal.mode != modalTaskView {
		t.Fatalf("enter should open the task popup, got %v", um.modal.mode)
	}
	if !strings.Contains(um.modal.vp.View(), "Popup task") {
		t.Error("popup should show the task detail")
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).modal.mode != modalNone {
		t.Error("esc should close the popup")
	}
}

// TestTaskPopupEditKey verifies 'e' in the popup closes it and starts the
// editor flow for the same task.
func TestTaskPopupEditKey(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Editable task")
	m := rootWithTask(t, dir, active)
	m.list.allTasks = []*models.Task{active}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um := updated.(Model)
	updated, cmd := um.Update(keyRune('e'))
	um = updated.(Model)
	if um.modal.mode != modalNone {
		t.Error("e should close the popup")
	}
	if cmd == nil {
		t.Fatal("e should produce the editor command")
	}
}

// TestCardTags verifies the @/!/& card tag computation.
func TestCardTags(t *testing.T) {
	dir := newTestBoard(t)
	if _, _, err := tasks.Create(dir, "base", "opus", nil, nil, ""); err != nil { // 1: @, & (2 and 3 depend on it)
		t.Fatal(err)
	}
	if _, _, err := tasks.Create(dir, "waiting", "", nil, []string{"1"}, ""); err != nil { // 2: ! (1 not done)
		t.Fatal(err)
	}
	if _, _, err := tasks.Create(dir, "met", "", nil, []string{"1"}, ""); err != nil { // 3: ! until 1 done
		t.Fatal(err)
	}
	snap, err := tasks.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	tags := cardTags(snap)
	if tags["1"] != "@&" {
		t.Errorf("task 1 tags = %q, want \"@&\"", tags["1"])
	}
	if tags["2"] != "!" || tags["3"] != "!" {
		t.Errorf("dependents tags = %q/%q, want \"!\"", tags["2"], tags["3"])
	}

	// Once the dependency is done, ! clears.
	if _, _, err := tasks.SetStatus(dir, "1", "done"); err != nil {
		t.Fatal(err)
	}
	snap, err = tasks.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	tags = cardTags(snap)
	if tags["2"] != "" || tags["3"] != "" {
		t.Errorf("met-dependency tags should be empty, got %q/%q", tags["2"], tags["3"])
	}

	// An archived dependency also counts as resolved, whatever its status.
	if _, _, err := tasks.SetStatus(dir, "1", "failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetState(dir, "1", "archive"); err != nil {
		t.Fatal(err)
	}
	snap, err = tasks.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	tags = cardTags(snap)
	if tags["2"] != "" || tags["3"] != "" {
		t.Errorf("archived-dependency tags should be empty, got %q/%q", tags["2"], tags["3"])
	}
}

// TestDoctorModalHealthy verifies that 'D' opens the doctor popup and shows a
// clean bill of health on a fresh board.
func TestDoctorModalHealthy(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Active task")

	m := rootWithTask(t, dir, active)
	updated, _ := m.Update(keyRune('x'))
	um := updated.(Model)
	if um.modal.mode != modalDoctor {
		t.Fatalf("expected doctor modal, got %v", um.modal.mode)
	}
	if !strings.Contains(um.modal.view(), "healthy") {
		t.Errorf("healthy board should say so: %q", um.modal.view())
	}
	// esc closes.
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).modal.mode != modalNone {
		t.Error("esc should close the doctor popup")
	}
}

// TestDoctorModalFix verifies the popup lists issues and that 'f' repairs
// them in place and reloads the board.
func TestDoctorModalFix(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Active task")
	// A CRLF task file is a fixable issue.
	crlf := dir + "/tasks/9-crlf.md"
	if err := os.WriteFile(crlf,
		[]byte("---\r\nid: \"9\"\r\ntitle: crlf\r\nstatus: ready\r\n---\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := rootWithTask(t, dir, active)
	updated, _ := m.Update(keyRune('x'))
	um := updated.(Model)
	if !strings.Contains(um.modal.view(), "crlf") {
		t.Fatalf("issue missing from report: %q", um.modal.view())
	}

	updated, cmd := um.Update(keyRune('f'))
	um = updated.(Model)
	if um.modal.mode != modalDoctor {
		t.Fatal("fix should keep the report open")
	}
	if !strings.Contains(um.modal.view(), "(fixed)") {
		t.Errorf("fix report should mark repairs: %q", um.modal.view())
	}
	if cmd == nil {
		t.Fatal("fix should trigger a reload")
	}
	if _, ok := cmd().(reloadMsg); !ok {
		t.Error("fix command should resolve to reloadMsg")
	}
	// The file really was repaired.
	data, err := os.ReadFile(crlf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\r") {
		t.Error("CRLF file not rewritten by the TUI fix")
	}
}

// TestYankCopiesID verifies y yanks the selected task's bare id and confirms
// in the status bar.
func TestYankCopiesID(t *testing.T) {
	dir := newTestBoard(t)
	active := mustActive(t, dir, "Active task")

	var got string
	orig := yankToClipboard
	yankToClipboard = func(text string) { got = text }
	defer func() { yankToClipboard = orig }()

	m := rootWithTask(t, dir, active)
	updated, _ := m.Update(keyRune('y'))
	um := updated.(Model)
	if got != active.ID {
		t.Errorf("yanked %q, want bare id %q", got, active.ID)
	}
	if um.notice.text != "yanked "+active.ID {
		t.Errorf("notice: %q", um.notice.text)
	}
}

// TestDepsModalBuild verifies the w picker's entry set: all states listed,
// archive last, self and cycle-creators greyed, resolved deps marked.
func TestDepsModalBuild(t *testing.T) {
	dir := newTestBoard(t)
	base := mustActive(t, dir, "Base")                                                   // 1
	if _, _, err := tasks.Create(dir, "Child", "", nil, []string{"1"}, ""); err != nil { // 2 depends on 1
		t.Fatal(err)
	}
	if _, _, err := tasks.Create(dir, "Old", "", nil, nil, ""); err != nil { // 3
		t.Fatal(err)
	}
	if _, err := tasks.SetState(dir, "3", "archive"); err != nil {
		t.Fatal(err)
	}

	m := rootWithTask(t, dir, base)
	updated, _ := m.Update(keyRune('w'))
	um := updated.(Model)
	if um.modal.mode != modalDepsPicker {
		t.Fatalf("expected deps picker, got %v", um.modal.mode)
	}
	byID := map[string]depEntry{}
	for _, e := range um.modal.entries {
		byID[e.id] = e
	}
	if e := byID["1"]; e.invalid != "self" {
		t.Errorf("task 1 should be greyed as self: %+v", e)
	}
	if e := byID["2"]; e.invalid != "cycle" {
		t.Errorf("task 2 (dependent) should be greyed as cycle: %+v", e)
	}
	if e := byID["3"]; e.invalid != "" || !e.resolved {
		t.Errorf("archived task 3 should be a selectable resolved candidate: %+v", e)
	}
	// Archive sorts last.
	last := um.modal.entries[len(um.modal.entries)-1]
	if last.id != "3" {
		t.Errorf("archive should sort last, got %q", last.id)
	}
}

// TestDepsModalToggleAndApply verifies space toggles a valid entry, space on
// an invalid one explains itself, and enter applies the checked set.
func TestDepsModalToggleAndApply(t *testing.T) {
	dir := newTestBoard(t)
	if _, _, err := tasks.Create(dir, "Dep", "", nil, nil, ""); err != nil { // 1
		t.Fatal(err)
	}
	work := mustActive(t, dir, "Work") // 2

	m := rootWithTask(t, dir, work)
	updated, _ := m.Update(keyRune('w'))
	um := updated.(Model)

	// Find task 1 among the visible rows and move the cursor onto it.
	vis := um.modal.visibleEntries()
	target := -1
	for vi, idx := range vis {
		if idx >= 0 && um.modal.entries[idx].id == "1" {
			target = vi
		}
	}
	if target < 0 {
		t.Fatal("task 1 not in the picker")
	}
	for um.modal.cursor < target {
		updated, _ = um.Update(keyRune('j'))
		um = updated.(Model)
	}
	updated, _ = um.Update(keyRune(' '))
	um = updated.(Model)
	if !um.modal.checked["1"] {
		t.Fatal("space should toggle the entry on")
	}

	updated, cmd := um.Update(tea.KeyMsg{Type: tea.KeyEnter})
	um = updated.(Model)
	if um.modal.mode != modalNone {
		t.Fatal("enter should close the picker")
	}
	if cmd == nil {
		t.Fatal("enter should produce an apply command")
	}
	if msg, ok := cmd().(actionDoneMsg); !ok || msg.notice.level != noticeSuccess {
		t.Fatalf("apply should succeed: %+v", msg)
	}
	got, err := tasks.Get(dir, "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "1" {
		t.Errorf("depends_on not applied: %v", got.DependsOn)
	}
}

// TestDepsModalInvalidToggleExplains verifies space on a greyed entry is a
// no-op with an in-modal explanation.
func TestDepsModalInvalidToggleExplains(t *testing.T) {
	dir := newTestBoard(t)
	base := mustActive(t, dir, "Base") // 1

	m := rootWithTask(t, dir, base)
	updated, _ := m.Update(keyRune('w'))
	um := updated.(Model)
	// Move to the first real entry (task 1 itself — greyed as self).
	updated, _ = um.Update(keyRune('j'))
	um = updated.(Model)
	updated, _ = um.Update(keyRune(' '))
	um = updated.(Model)
	if um.modal.checked["1"] {
		t.Error("space must not toggle an invalid entry")
	}
	if um.modal.note == "" {
		t.Error("expected an in-modal explanation")
	}
}

// TestDepsModalFilterAndClear verifies the in-modal / filter narrows entries
// and the clear-all row empties the checked set.
func TestDepsModalFilterAndClear(t *testing.T) {
	dir := newTestBoard(t)
	if _, _, err := tasks.Create(dir, "alpha", "", nil, nil, ""); err != nil { // 1
		t.Fatal(err)
	}
	if _, _, err := tasks.Create(dir, "beta", "", nil, nil, ""); err != nil { // 2
		t.Fatal(err)
	}
	work := mustActive(t, dir, "Work") // 3

	m := rootWithTask(t, dir, work)
	updated, _ := m.Update(keyRune('w'))
	um := updated.(Model)

	// Filter to "alpha": only the pinned clear-all row plus task 1 remain.
	updated, _ = um.Update(keyRune('/'))
	um = updated.(Model)
	for _, r := range "alpha" {
		updated, _ = um.Update(keyRune(r))
		um = updated.(Model)
	}
	updated, _ = um.Update(tea.KeyMsg{Type: tea.KeyEnter}) // leave the input, keep the filter
	um = updated.(Model)
	vis := um.modal.visibleEntries()
	if len(vis) != 2 || um.modal.entries[vis[1]].id != "1" {
		t.Fatalf("filter should narrow to task 1: %v", vis)
	}

	// Clear-all: check task 1 first, then clear from the pinned row.
	updated, _ = um.Update(keyRune('j'))
	um = updated.(Model)
	updated, _ = um.Update(keyRune(' '))
	um = updated.(Model)
	if !um.modal.checked["1"] {
		t.Fatal("toggle under filter failed")
	}
	updated, _ = um.Update(keyRune('g')) // jump to the clear-all row
	um = updated.(Model)
	updated, _ = um.Update(keyRune(' '))
	um = updated.(Model)
	if len(um.modal.checked) != 0 {
		t.Errorf("clear all should empty the set: %v", um.modal.checked)
	}
}

// TestSetThemeRoundTrip verifies switching to a preset changes the active
// theme and switching back restores the default palette.
func TestSetThemeRoundTrip(t *testing.T) {
	t.Cleanup(func() { setTheme("") })

	defDim := th.ID.GetForeground()
	if w := setTheme("high-contrast"); w != "" {
		t.Fatalf("high-contrast should be a known preset, got warning %q", w)
	}
	if got := th.ID.GetForeground(); got == defDim {
		t.Error("high-contrast should change the dim (ID) colour")
	}
	if w := setTheme(""); w != "" {
		t.Fatalf("empty name should select default, got warning %q", w)
	}
	if got := th.ID.GetForeground(); got != defDim {
		t.Errorf("empty name should restore the default palette, got %v", got)
	}
}

// TestSetThemeUnknown verifies an unknown preset name warns and leaves the
// default palette installed.
func TestSetThemeUnknown(t *testing.T) {
	t.Cleanup(func() { setTheme("") })

	defDim := defaultTheme().ID.GetForeground()
	w := setTheme("nope")
	if !strings.Contains(w, `unknown theme "nope"`) {
		t.Errorf("warning should name the unknown theme, got %q", w)
	}
	if got := th.ID.GetForeground(); got != defDim {
		t.Errorf("unknown theme should leave the default palette, got %v", got)
	}
}

// TestNewModelThemeWarning verifies an unknown theme in Options surfaces as
// the initial warn-level status-bar notice.
func TestNewModelThemeWarning(t *testing.T) {
	t.Cleanup(func() { setTheme("") })

	m := NewModel(t.TempDir(), Options{Theme: "nope"})
	if m.notice.level != noticeWarn {
		t.Fatalf("initial notice should be warn-level, got %v: %q",
			m.notice.level, m.notice.text)
	}
	if !strings.Contains(m.notice.text, `unknown theme "nope"`) {
		t.Errorf("notice should name the unknown theme, got %q", m.notice.text)
	}
	m.width, m.height = 80, 24
	if !strings.Contains(m.View(), "unknown theme") {
		t.Error("initial view should render the theme warning")
	}
}

// styleChrome renders a style's non-color attributes — border glyphs and
// edges, bold flag, padding — as a comparable string.
func styleChrome(s lipgloss.Style) string {
	top, right, bottom, left := s.GetPadding()
	return fmt.Sprintf("border=%v edges=%v/%v/%v/%v bold=%v padding=%d/%d/%d/%d",
		s.GetBorderStyle(),
		s.GetBorderTop(), s.GetBorderRight(), s.GetBorderBottom(), s.GetBorderLeft(),
		s.GetBold(), top, right, bottom, left)
}

// themeChrome renders every style in a theme through styleChrome, keyed by
// field name so mismatches point at the offending style.
func themeChrome(tm theme) string {
	named := []struct {
		name string
		s    lipgloss.Style
	}{
		{"ColumnActive", tm.ColumnActive},
		{"ColumnInactive", tm.ColumnInactive},
		{"CardSelected", tm.CardSelected},
		{"CardNormal", tm.CardNormal},
		{"ID", tm.ID},
		{"PaneActive", tm.PaneActive},
		{"PaneInactive", tm.PaneInactive},
		{"Modal", tm.Modal},
		{"Header", tm.Header},
		{"Footer", tm.Footer},
		{"StateDraft", tm.StateDraft},
		{"StateArchive", tm.StateArchive},
		{"NoticeSuccess", tm.NoticeSuccess},
		{"NoticeWarn", tm.NoticeWarn},
		{"NoticeError", tm.NoticeError},
		{"Help", tm.Help},
		{"HelpKey", tm.HelpKey},
		{"HelpDesc", tm.HelpDesc},
	}
	var sb strings.Builder
	for _, n := range named {
		fmt.Fprintf(&sb, "%s: %s\n", n.name, styleChrome(n.s))
	}
	statuses := make([]string, 0, len(tm.Status))
	for k := range tm.Status {
		statuses = append(statuses, k)
	}
	sort.Strings(statuses)
	for _, k := range statuses {
		fmt.Fprintf(&sb, "Status[%s]: %s\n", k, styleChrome(tm.Status[k]))
	}
	return sb.String()
}

// TestThemePresetsAreColorsOnly pins the project invariant that theme presets
// differ in colors only: every preset must share the default's border glyphs,
// bold flags, and padding on every style.
func TestThemePresetsAreColorsOnly(t *testing.T) {
	want := themeChrome(buildTheme(defaultPalette()))
	presets := map[string]theme{
		"saturated":     buildTheme(saturatedPalette()),
		"high-contrast": buildTheme(highContrastPalette()),
	}
	for name, tm := range presets {
		if got := themeChrome(tm); got != want {
			t.Errorf("preset %q changes non-color chrome:\n got: %s\nwant: %s", name, got, want)
		}
	}
}

// TestNewModelThemeWarningPrecedence verifies a pre-resolved config warning
// outranks the bad-theme-name warning.
func TestNewModelThemeWarningPrecedence(t *testing.T) {
	t.Cleanup(func() { setTheme("") })

	m := NewModel(t.TempDir(), Options{Theme: "nope", ThemeWarning: "config unreadable"})
	if m.notice.level != noticeWarn || m.notice.text != "config unreadable" {
		t.Errorf("ThemeWarning should win, got %v: %q", m.notice.level, m.notice.text)
	}
}

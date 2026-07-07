# 044 — TUI Board Layout: state columns + task popup

Decisions (user-resolved, 2026-07-05):

- Board shows **all seven columns**: `draft | ready | in_progress | blocked |
  done | failed | archive`. State columns flank the status columns; the status
  display order changes to flow order (blocked next to in_progress, terminal
  states last) — `models.Statuses` reordered globally for consistency (CLI
  board/picker order follows).
- Cards in the two state columns carry a **status-colored dot** (`● 12 title`);
  status columns don't need one.
- Drafts sort oldest-first (FIFO triage); archive sorts newest-first.
- **Enter on a board card opens a centered read-only popup** (same content as
  the list detail pane: meta + deps + rendered body; j/k scrolls, `e` edits,
  esc closes) instead of jumping to the list view.
- List view stays as the secondary flat/search view (unchanged).
- Footer loses the draft/archive counts (they're columns now) and **wraps to
  extra lines on narrow terminals** instead of overflowing; body height adjusts.
- `/` stays pure search (no state cycling); no empty-column collapsing; no
  filter-matching on state/status words; no dep glyphs (deferred).

## Files

| Change | Files |
|---|---|
| Status flow order | `internal/models/models.go`, docs/skill listings |
| 7-column board, dot, sorts, footer | `internal/tui/board.go` |
| Task-view popup modal | `internal/tui/modal.go`, `internal/tui/model.go` |
| Root footer wrapping + heights | `internal/tui/model.go`, `internal/tui/list.go` |
| Tests | `internal/tui/tui_test.go` |
| Docs | `README.md` (TUI section, status order), `docs/design/02_board-data-model.md` |

## Todo

1. [x] `models.Statuses` → ready, in_progress, blocked, done, failed.
2. [x] Board: seven columns (state cols flanking), dot on state-column cards,
   draft asc / archive desc, empty hint rewording, drop count footer fields.
3. [x] Root-owned wrapped footer (shared helper), body height accounts for
   footer height at the current width.
4. [x] `modalTaskView` popup: viewport, j/k + g/G scrolling, esc closes,
   `e` closes + opens the editor; board Enter opens it (list Enter unchanged).
5. [x] Tests: 7-column loadData, popup open/close/edit, footer wrap.
6. [x] Docs: README TUI section + status order listings; 02 status table order.
7. [x] Gate: make vet && make test.

## Change history
- [2026-07-05] Plan written.
- [2026-07-05] Implemented in full; gate green.

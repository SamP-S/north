# 043 — TUI UX Improvements (pre-v1.0)

Decisions from the TUI UX review (2026-07-05), all resolved with the user:

- Board columns must scroll (defect: cards beyond the column height are clipped
  while the cursor can still reach them).
- Search is global: `/` works on the board too and filters the columns in
  place (cursor clamps to remaining cards); esc clears the filter. Search
  state is hoisted from the list sub-model into the root model.
- Status becomes freeform **everywhere**: `tasks.SetStatus` drops the
  active-only guard; CLI `task move` and the TUI `m` picker work in any state,
  with a warning on non-active tasks (status only shows on the board once
  active). Docs/skill/CLAUDE.md updated.
- New notice bar just above the footer in both views: green success
  ("1 → done"), yellow warnings, red errors. Replaces the current
  error-only line. Cleared on next keypress.
- Delete confirm accepts enter as well as y.
- g/G jump to top/bottom in list and board columns (no pgup/pgdn).
- Empty-board hint when there are no active tasks and no filter.
- TUI is keyboard-only — rigid design decision, documented; no mouse support.
- README: note that `go install` requires `$GOPATH/bin` on PATH.

## Files

| Change | Files |
|---|---|
| Freeform status | `internal/tasks/tasks.go`, `internal/tasks/guards_test.go`, `internal/cli/task.go`, `internal/cli/cli_test.go` |
| Docs for the above | `CLAUDE.md`, `README.md`, `docs/design/02_board-data-model.md`, `docs/design/03_cli.md`, `internal/skill/skill/SKILL.md` |
| Notice bar + shared search + g/G + column scroll + empty hint + enter-confirm | `internal/tui/{model,board,list,modal,actions,keys,styles}.go`, `internal/tui/tui_test.go` |
| Keyboard-only note | `docs/design/01_overview.md` or TUI section, `docs/design/99_roadmap.md` (drop mouse ideas) |
| PATH note | `README.md` |

## Todo

1. [x] Core: `tasks.SetStatus` freeform across states; guards tests updated.
2. [x] CLI: `task move` works in any state; stderr note when task not active;
   help text + cli tests updated.
3. [x] TUI root: notice bar (success/warn/error levels, styles), rendered above
   the footer in both views; `runTaskOp` carries a success message; warning
   notice when `m` used on a non-active task.
4. [x] TUI root: hoist search (`/`) — input + query owned by root, rendered
   above the footer; filters both board columns and list rows; esc clears;
   cursor clamps.
5. [x] Board: column scroll windowing (cursor always visible).
6. [x] Board: empty-state hint; list: keep behaviour.
7. [x] Keys: g/G top/bottom (board column + list); delete confirm accepts enter.
8. [x] Docs: freeform status everywhere; keyboard-only decision; README PATH
   note; SKILL.md status wording; CLAUDE.md project line.
9. [x] Tests + `make vet && make test` green; e2e smoke of move-on-draft via CLI.

## Change history
- [2026-07-05] Plan written; all decisions pre-resolved with user.
- [2026-07-05] Implemented in full: freeform status everywhere (+CLI stderr note), root-owned notice bar + global filter, board column scrolling, g/G, enter-confirm delete, empty-board hints, docs/skill/README/CLAUDE.md updated (incl. go install PATH note), keyboard-only recorded in roadmap.

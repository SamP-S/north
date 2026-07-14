# 052 — Fix `next/take --plain` row format and delete TTY detection

## Summary

Three issues found while dogfooding the skill on the populated calculator board:

1. `north next` / `north take` with `--plain` print a multi-line detail record
   (via `render.TaskDetail`), unlike every other `--plain` list surface which
   emits one tab-separated row. Scripts cannot line-parse the pick result.
2. `north task delete <id>` without `-y` and with non-TTY stdin (e.g.
   `/dev/null` under an agent harness) shows the interactive `[y/N]` prompt and
   auto-aborts instead of hitting the intended
   "delete requires -y when not run interactively" guard. Root cause:
   `stdinIsTTY` checks `os.ModeCharDevice`, and `/dev/null` is a char device.
3. A declined/aborted delete exits 1 (internal) — `Execute` hardcodes it —
   while `errAborted` is a conflict; the documented contract says conflict = 4.

## Files to modify

- `internal/cli/next.go` — `printPickResult` plain case renders a single
  `render.TaskList` row instead of `render.TaskDetail`.
- `internal/cli/task.go` — `stdinIsTTY` uses `golang.org/x/term.IsTerminal`
  (already in the module graph, promoted to a direct dependency).
- `internal/cli/root.go` — aborted branch returns `nerrors.ExitCode(err)`
  (4, conflict) instead of hardcoded 1; still prints no extra error line.
- `internal/cli/next_test.go` — assert the single-row `--plain` shape.
- `internal/cli/cli_test.go` — non-TTY delete without `-y` is refused with an
  invalid error; declined delete exits with the conflict code.
- `go.mod` — `golang.org/x/term` becomes a direct dependency.

## Todo

1. [x] Plain pick row: `printPickResult` uses `render.TaskList` (single row).
2. [x] `stdinIsTTY` via `term.IsTerminal`; keep non-file readers = interactive.
3. [x] `errAborted` exits with its conflict code (4).
4. [x] Update/add tests for all three.
5. [x] `make fmt vet test build` clean; manual verification against a board.

## Change history

- [2026-07-14] Plan created; all three fixes implemented and verified.

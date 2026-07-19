# 060 — third cleaning pass

## Summary

Fixes for the third full-source audit (post-059, commit b53b9aa). Every finding
was read-verified; F1 and F2 were also reproduced on a scratch board. Scope
decided with the user [2026-07-19]:

- **F1 — doctor --fix single-run heal for duplicate+dangling files.** The
  duplicate-id repair renames the file, but `tasksParsed` keeps the
  pre-renumber path/id, so the dangling-dep repair `loadTask`s a removed path
  and silently fails (needs a second `--fix` run); the cycle report can also
  name stale ids. Fix: after a successful renumber, update the corresponding
  `tasksParsed` entry's path and id.
- **F2 — unparseable timestamps are preserved, and doctor flags them.**
  `parseDT` nils any non-conforming `created_at`/`updated_at` and the next
  rewrite persists `null`, destroying the original value. Fix: when the task's
  time is nil but the previous frontmatter node holds a non-empty, non-null
  value, `renderTask` leaves that node untouched (the unknown-key treatment);
  doctor gains a report-only `bad-timestamp` issue kind for non-empty
  unparseable timestamp values.
- **F3 — `--limit` mimics `git log -n`: N means at most N, including 0.**
  Reverses 058-B11. `task list -l 0` and `next -l 0` print zero rows (exit 0,
  empty plain output, `{"tasks": []}` under `--json`); omitting the flag keeps
  today's defaults (list = all, next = 1); negatives stay invalid. "0 = all"
  is gone — users who want everything omit `--limit`. Implemented via
  `Flags().Changed`; the library's `limit <= 0 = all` semantics are untouched,
  the CLI truncates. Help text + SKILL.md + README + 03_cli.md updated
  (03_cli.md line 35 still documented the pre-058 "≥ 2 / < 1 invalid"
  contract — that drift dies here too).
- **F4 — status-mutability doc drift.** `models.go` ("It only changes while
  the task is active.") and SKILL.md ("only meaningful (and only changeable)
  while the task is **active**") contradict actual behaviour (settable in any
  state, advisory warning). Reword both.
- **F5 — TUI delete confirm: only `y` confirms.** `enter` currently confirms
  against a `[y/n]` prompt (CLI's default is No). New behaviour: `y` confirms,
  `n`/`esc` cancel, everything else (incl. enter) is inert.
- **F6 — unify I/O error codes.** `readBody` wraps `os.ReadFile`/stdin
  failures in `Invalid` (exit 2); the 058-A4 rule is I/O = internal (exit 1),
  usage = 2. Let read failures pass through; keep `Invalid` for the
  `--body`+`--body-file` combination. Audit for other I/O→Invalid mappings
  while there.
- **F7 — `config set` validates before locking.** Key/value validation
  (read-only keys, parse errors) moves above `board.Lock`; the malformed-config
  gate stays under the lock.
- **F8 — `tui` help string lowercased** ("open the interactive terminal UI")
  to match every sibling `Short`.
- **F10 — `skill check` is doctor-style: exit 0 when the check completes.**
  Drop the Conflict(4)/outdated and NotFound(3)/all-missing returns; the
  per-target report (human lines, `--json` targets array) is the output.
  Contract documented in 03_cli.md.
- **F12 — `take` empty-assignee message reworded** to "assignee must not be
  empty (pass --assignee or set NORTH_AGENT)" — accurate for both the
  explicit `--assignee ""` and unset cases. Empty stays invalid on `take`
  (claiming needs a claimant; unsetting lives on `edit --assignee ""`).

Declined/absorbed: F9 (whitespace `--append-body` no-op — keep as is, user
decision), F11 (stale cycle ids — absorbed by F1).

## Files to modify

| Finding | Files |
|---|---|
| F1 | `internal/tasks/doctor.go`, `internal/tasks/doctor_test.go` |
| F2 | `internal/tasks/frontmatter.go` (renderTask), `internal/tasks/doctor.go`, parse/edit/doctor tests |
| F3 | `internal/cli/task.go`, `internal/cli/next.go`, `internal/cli/cli_test.go`, `internal/cli/next_test.go`, `internal/skill/skill/SKILL.md`, `README.md`, `docs/design/03_cli.md` |
| F4 | `internal/models/models.go`, `internal/skill/skill/SKILL.md` |
| F5 | `internal/tui/modal.go`, `internal/tui/tui_test.go` |
| F6 | `internal/cli/task.go` (readBody), `internal/cli/cli_test.go` |
| F7 | `internal/cli/config.go` |
| F8 | `internal/cli/tui.go` |
| F10 | `internal/cli/skill.go`, `internal/cli/cli_test.go` (or skill tests), `docs/design/03_cli.md` |
| F12 | `internal/tasks/next.go`, tests touching the message |

## Todo

1. [x] Write this plan
2. [x] Wave 1 — tasks core: F1, F2, F12 (+tests: one-run duplicate+dangling
       heal; bad-timestamp preservation through edit; doctor bad-timestamp
       issue; take message)
3. [x] Wave 2 — cli: F3, F6, F7, F8, F10 (+tests: `-l 0` on list/next in all
       three output modes; body-file I/O exit 1; skill check exit 0 when
       outdated/missing)
4. [x] Wave 3 — tui: F5 (+test: enter is inert on the delete confirm)
5. [x] Docs: SKILL.md / README / 03_cli.md for F3, F4, F10
6. [x] Verify: `make build vet` + `go test ./...` + scratch-board probes
       (doctor one-run heal, timestamp preservation, `-l 0`, body-file perm
       error, skill check exit codes)

## Change history

- [2026-07-19] Audit completed (full source read, F1/F2 reproduced); findings
  triaged with the user; plan written. No code changes yet.
- [2026-07-19] All fixes implemented via four parallel sub-agents (tasks, cli,
  tui, docs). F1: `tasksParsed` path/id updated after renumber (one-run heal;
  stale cycle ids gone). F2: `renderTask` preserves non-empty unparseable
  timestamp nodes via `setTimeKey`; doctor gains report-only `bad-timestamp`.
  F3: `--limit` keys off `Flags().Changed` — `-l 0` = zero rows on list and
  next, omit for defaults, negatives invalid; help/Long/docs updated. F5: TUI
  delete confirm — only `y` confirms, enter inert. F6: `readBody` I/O failures
  pass through as internal (exit 1). F7: `config set` validates before
  locking. F8: `tui` Short lowercased. F10: `skill check` always exits 0 on a
  completed check. F12: take message reworded. Tests added/updated in
  doctor_test, parse_test, cli_test, next_test, tui_test.
- [2026-07-19] Verify done: `make build vet`, `go test ./...`, `gofmt -l` all
  clean; scratch-board probes confirmed every item — one-run duplicate+dangling
  heal then clean second scan, bad-timestamp flagged and preserved through
  edit, `list -l 0`/`next -l 0` empty exit 0 vs `-l -1` invalid, body-file
  directory read → internal exit 1, skill check exit 0 with missing targets,
  `config set last_id` refused without touching `.lock`, reworded take
  message. Plan complete.

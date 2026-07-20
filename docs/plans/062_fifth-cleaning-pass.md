# 062 — fifth cleaning pass

## Summary

Fixes for the fifth full-source audit (post-061, commit 1942227). Every
finding below was reproduced on a scratch board before being written down, and
cross-checked against docs/design and plans 037–061 so nothing already settled
is re-litigated. Triaged with the user [2026-07-20]: fix all ten.

Two principles carried forward from 061 govern the ambiguous cases:

- **Broken-file principle.** If an operation only touches parts of a file that
  are still valid, it proceeds and warns (recommending `north doctor`); if the
  part being operated on is itself broken or missing, that operation refuses
  rather than guessing. This decides G6.
- **One logical command, one commit.** Established for `doctor --fix` in 061's
  F3; G3 applies it to `delete`.

- **G1 — a corrupt `last_id` must be a hard error.** `LoadConfig` rejects a
  malformed `auto_commit`, `max_wip`, or `deps_enforcement` but silently
  falls back to 0 for `last_id`, so the file scan alone decides the next id.
  Verified: with `last_id: garbage`, deleting task 2 and creating a new task
  reissued id 2 — the exact hole the high-water mark exists to close, which
  the README and `02_board-data-model.md` both promise cannot happen. This is
  plan 057's item A2 ("a silently ignored typo must not silently change
  behaviour") applied to the one key it was never applied to. Fix: when the
  key is present it must be a non-negative integer, else `Invalid` naming the
  file and value. Absent stays 0 (pre-mark boards).
- **G2 — board-owned file writes must be atomic.** `setConfigKey` ends in a
  bare `os.WriteFile`, truncating config.yml in place. It is on the hot path
  (every `create` bumps `last_id`), and config.yml is both the discovery
  marker and a hard error when malformed, so a crash or ENOSPC mid-write
  bricks the board. Fix: one `writeFileAtomic` helper (temp + rename, temp
  named `*.tmp` so the board `.gitignore` already covers it), used by
  `setConfigKey`, `WriteGitattributes`, `WriteGitignore`, and the config and
  template writes in `InitBoard`.
- **G3 — `delete` makes one commit.** Deleting a task with three dependents
  produced four commits (`north: delete 1`, then one `heal deps of …` each),
  and a failure part-way leaves the removal committed with only some
  dependents healed. Fix: mirror 061's doctor change — heal through `persist`
  (and the surgical rewrite) without committing, collect every changed and
  removed path, and make a single `north: delete <id>` commit at the end.
  A final-commit failure is returned as an error.
- **G4 — usage validation happens before board discovery, everywhere.** 061's
  F9 fixed `task list` only; `next` and `cleanup` were already correct, so
  the CLI is currently split three ways. Verified from a boardless directory,
  these nine report `not_found` (exit 3) instead of the usage error (exit 2)
  they give inside a board: `create --body a --body-file b`; `edit --body a
  --append-body b`; `move`/`state`/`delete` with an empty id list;
  `move`/`state` with an unknown status/state; `take <id> --label x`;
  `config set` and `config get` with an unknown key. Fix: move each check
  ahead of `board.LocateBoard`. `config get` needs its key checked against
  `configKeys` before the config load, not inside `configTypedValue`.
- **G5 — batch `move`/`state` validate the status once.** `task move 2,3,4
  bogus` prints the same "unknown status" error three times plus a summary.
  The status is a single positional, not per-id. Fix: parse it once, before
  the loop (and before `LocateBoard`, per G4); the per-id parse inside
  `SetStatus`/`SetState` stays as the library-level guard.
- **G6 — `cleanup --older-than` warns about undated tasks instead of
  silently skipping them.** Verified: a done task with no `updated_at` is
  archived by a plain `cleanup` but invisible to `cleanup --older-than 1`,
  with no warning, permanently. The part being operated on (the timestamp) is
  the missing part, so per the broken-file principle the task is correctly
  skipped — but it must say so. Fix: keep skipping, and emit a warning naming
  the file ("has no readable updated_at — cannot judge its age; run `north
  doctor`"). Warnings ride the existing `[]Warning` return, so they reach
  stderr in human/plain and the `warnings` array under `--json`.
- **G7 — three rough edges in `healBrokenDependents` (new in 061).**
  (a) It calls a duplicate-id file "broken", though such a file is
  well-formed and its only problem is a shared id; (b) it skips the
  `updated_at` bump, correct for a genuinely unparseable file but wrong for a
  merely duplicated one; (c) `findTaskFile` resolves warnings by base name
  across all three state folders, so two same-named files — the realistic
  duplicate-id case, `drafts/1-a.md` and `tasks/1-a.md` — can route a heal to
  the wrong file. Fix: give `Warning` a `Kind` ("unparseable" | "duplicate-id")
  and a `Path`, set by `Load`. `Warning.String()` keeps its current
  `file: err` form so no output changes. Duplicate-id files then heal through
  the ordinary `loadTask` + `persist` path (bump included, wording about a
  duplicate id, not brokenness); unparseable files keep the surgical
  node edit; and `findTaskFile` is deleted in favour of `Warning.Path`.
- **G8 — `doctor --help` is stale.** Its `Long` says `--fix` applies "CRLF
  rewrite, duplicate renumbering, filename renames", omitting dangling-dep
  removal and the `.gitattributes`/`.gitignore` restore, and never mentions
  the single commit `03_cli.md` now documents.
- **G9 — `--plain` with `--json` is a usage error.** Every command silently
  prefers JSON. The docs write the pair as `--plain | --json` throughout but
  never state a precedence, so today's behaviour is undocumented rather than
  wrong. Fix: reject the combination once, in a root `PersistentPreRunE`, as
  `Invalid` (exit 2) — provided no subcommand defines its own
  `PersistentPreRunE` that would shadow it.
- **G10 — `skill install --target` deduplicates.** `selectAgents` does not,
  so `--target claude --target claude` writes and reports the target twice
  (and `skill check` would list it twice). Fix: dedupe while preserving order.

## Files to modify

| Finding | Files |
|---|---|
| G1 | `internal/board/board.go` (LoadConfig), `internal/board/board_test.go` |
| G2 | `internal/board/board.go` (writeFileAtomic, setConfigKey, Write*, InitBoard), `internal/board/board_test.go` |
| G3 | `internal/tasks/tasks.go` (Delete, healBrokenDependents), `internal/tasks/deps_test.go` |
| G4 | `internal/cli/task.go`, `internal/cli/next.go`, `internal/cli/config.go`, `internal/cli/cli_test.go` |
| G5 | `internal/cli/task.go`, `internal/cli/cli_test.go` |
| G6 | `internal/tasks/tasks.go` (Cleanup), `internal/tasks/archive_test.go` |
| G7 | `internal/tasks/snapshot.go` (Warning), `internal/tasks/tasks.go`, `internal/tasks/deps_test.go` |
| G8 | `internal/cli/doctor.go` |
| G9 | `internal/cli/root.go`, `internal/cli/cli_test.go` |
| G10 | `internal/skill/install.go`, `internal/skill/skill_test.go` |
| docs | `docs/design/05_configuration.md`, `03_cli.md`, `02_board-data-model.md`, `internal/skill/skill/SKILL.md` |

## Todo

1. [x] Write this plan
2. [x] Wave 1 — board/config: G1, G2 (+tests: malformed `last_id` is
       `invalid`; absent stays 0; a config write leaves no `.tmp` behind and
       preserves comments/unknown keys)
3. [x] Wave 2 — tasks: G3, G6, G7 (+tests: one commit per delete with
       dependents; undated done task warns under `--older-than`; duplicate-id
       dependent heals with an `updated_at` bump; same-named files in two
       state folders heal the right one)
4. [x] Wave 3 — cli/skill: G4, G5, G8, G9, G10 (+tests: each usage error
       fires without a board; one error for a bad batch status; `--plain
       --json` is invalid; duplicate `--target` installs once)
5. [x] Wave 4 — docs: `last_id` hard error, `--plain`/`--json` exclusivity,
       `cleanup --older-than` warning, delete's single commit
6. [x] Verify: `make build vet` + `go test ./...` + scratch-board probes of
       all ten fixes end to end

## Change history

- [2026-07-20] Fifth audit completed (full non-TUI source read; all ten
  findings reproduced on scratch boards); cross-checked against plans 037–061;
  user triaged "fix all"; plan written. No code changes yet.
- [2026-07-20] All ten implemented across four parallel waves.
  - G1: `LoadConfig` reuses the `toInt(v, -1)` sentinel, so an unparseable or
    negative `last_id` is `Invalid`; absent and `0` are still fine.
  - G2: new `writeFileAtomic` (temp + rename, temp removed on either failure)
    backs `setConfigKey`, `WriteGitattributes`, `WriteGitignore`, and the
    config/template writes in `InitBoard`.
  - G3: `Delete` accumulates changed/removed paths and commits once; heals go
    through `persist`, and `healBrokenDependents` returns its paths instead of
    committing.
  - G4/G5: usage validation precedes `LocateBoard` in all nine paths, and the
    `move`/`state` positional is parsed once rather than per id.
  - G6: `Cleanup` splits the nil-`UpdatedAt` case out of the cutoff test and
    appends a `bad-timestamp` warning instead of skipping silently.
  - G7: `Warning` gained `Kind` and `Path` (`String()` unchanged, so no output
    moved); `findTaskFile` deleted; duplicate-id dependents heal normally with
    an `updated_at` bump, unparseable ones keep the surgical node edit.
  - G8/G9/G10: doctor help rewritten, `--plain`+`--json` rejected in a root
    `PersistentPreRunE`, `selectAgents` dedupes.
- [2026-07-20] Verification: `make build vet` clean, `go test ./...` all green,
  and scratch-board probes confirmed each fix end to end — all ten boardless
  usage cases exit 2, one commit per delete with a clean tree, one error for a
  bad batch status, corrupt `last_id` refused instead of reissuing an id, no
  `.tmp` left after a config write, the undated task warned under
  `--older-than`, and a duplicated `--target` installing once.
  **One regression was caught during review and fixed.** Wave 2's G7 healed
  duplicate-id dependents via `persist`, whose target path is recomputed from
  id + title. For a duplicate-id file that *also* has filename drift, that
  path is the namesake task's own file: a scratch probe showed
  `north task delete 3` overwriting `1-a.md` with the duplicate's content and
  deleting `2-a.md` — two tasks lost. Heals must never move a file, precisely
  because these files are snapshot-excluded for having an untrustworthy id.
  Both heal branches now write through a new in-place `writeTaskInPlace`, and
  `healBrokenDependents` no longer reports removed paths (it can no longer
  remove any). Guarded by `TestDeleteHealDoesNotClobberIDNamesake`.

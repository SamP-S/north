# 054 — CLI consistency pass

## Summary

Findings from a full reading pass of the CLI surface (2026-07-14), agreed with
Sam. Three behavioural fixes, four output-consistency fixes, and three small
refinements. Config commands stay as they are (config is deliberately separate
from the board: no lock, no `--plain`/`--json`). A full docs pass follows as
separate work once these land.

## Scope

### A. Behavioural fixes

1. **Stop task-id reuse** — `board.NextID` is "max id on disk + 1", so
   deleting the highest task frees its id for the next create. Add a
   `last_id` high-water mark to `north/config.yml`:
   - `NextID` returns `max(scan, last_id) + 1`; every allocation (create,
     doctor duplicate renumbering) persists the new high-water mark under the
     already-held board lock.
   - `last_id` is read-only via `config set` (like `version`) and listed by
     `config list`/`get`.
   - Missing key (older boards) falls back to scan-only — no migration step.
   - Auto-commit: the config.yml bump rides in the same commit as the created
     task file.
   - The config rewrite must preserve user comments: update via a yaml.Node
     round-trip (same approach as task frontmatter) rather than re-marshalling
     the struct, so init's commented scaffold survives creates. `config set`
     keeps its existing rewrite-plain behaviour.
   - Correct the `NextID` doc comment either way.

2. **Lock: extend `staleAfter` 10s → 120s** — a slow mutation (auto-commit
   hooks/signing, multi-task cleanup) can outlive 10s and have its lock stolen
   mid-write. Deliberately *not* adding mtime refresh or ownership tokens —
   keep the lock simple; 120s is expected to cover realistic slow operations.
   Trade-off accepted: a crashed process blocks writers for up to 2 minutes
   before self-healing (the error message already names the lock file for
   manual deletion).

3. **`errors.As` must unwrap** — it is a direct type assertion today, so any
   `%w`-wrapped BoardError degrades to internal/exit 1. Reimplement with
   stdlib `errors.As` semantics.

### B. Output consistency

4. **Single-target mutations print one row under `--plain`** — create, edit,
   move, state, delete currently print the multi-line `TaskDetail` record for
   one id but list rows for a batch. Standardise on the `task list --plain`
   row shape (id, state, status, assignee, labels, title) for all mutation
   results in plain mode. `view --plain` keeps the detail record (that is its
   job); `--json` payloads are unchanged.

5. **Empty `list --plain` prints nothing** — today it prints a lone newline
   (`cmd.Println` of an empty string), unlike empty `next --plain`. Suppress
   the write when the rendered output is empty.

6. **`cleanup --json` carries `"dry_run": true|false`** — the preview and the
   real run currently emit identical payloads; agents cannot confirm from
   output whether archiving happened. Human output already differs and stays
   as is.

7. **`doctor` exits 0 when the scan completes** — issues found are the
   command's *output*, not its failure. Drop the exit-4-on-unfixed-issues
   behaviour; CI/agents gate on the `--json` issues array instead. (Filesystem
   or board-location failures still error normally.)

### C. Refinements

8. **`git.CommitBoard` warning matches convention** — replace `log.Printf`
   (timestamped) with a plain `warning: …` line on stderr.

9. **Deduplicate helpers** — `joinKeys` (config.go) becomes `strings.Join`;
   label matching exists twice (`hasLabels` in tasks/next.go, `filterLabels`'s
   inner loop in cli/task.go) — export one from `tasks` and use it in both.

10. **`task list --limit N`** — cap the number of rows after filter+sort
    (0 = unlimited, the default), symmetric with `next -l`.

11. **SKILL.md documents the new surface** (added after implementation) —
    `list -l/--limit`, the `cleanup --json` `dry_run` key, doctor's
    exit-0-with-findings contract, and the read-only `last_id` config key.
    The full docs pass (CLAUDE.md wording etc.) remains separate follow-up.

## Files to modify

- `internal/board/board.go` — Config.LastID, NextID, comment-preserving
  config update helper, doc comment fix.
- `internal/board/lock.go` — staleAfter constant + comment.
- `internal/errors/errors.go` — unwrapping As.
- `internal/tasks/tasks.go` — id allocation persists last_id; Cleanup
  unchanged otherwise.
- `internal/tasks/doctor.go` — renumbering allocates through the same path.
- `internal/tasks/next.go` — export label matcher.
- `internal/cli/task.go` — printTaskResult plain row; empty-list suppression;
  --limit flag; use exported label matcher.
- `internal/cli/board.go` — cleanup dry_run key in JSON.
- `internal/cli/doctor.go` — exit 0 on completed scan.
- `internal/cli/config.go` — last_id read-only key; strings.Join.
- `internal/git/git.go` — warning format.
- Tests alongside every change (cli_test.go, next_test.go, board_test.go,
  lock_test.go, errors_test.go, tasks tests).

## Todo

1. [x] last_id: Config field + comment-preserving write + NextID/allocation +
       config list/get/set handling + auto-commit inclusion + tests.
2. [x] staleAfter → 120s + comment + lock_test adjustments.
3. [x] errors.As unwraps + test with a %w-wrapped BoardError.
4. [x] Single-mutation --plain row output + test updates.
5. [x] Empty list --plain prints nothing + test.
6. [x] cleanup --json dry_run key + test.
7. [x] doctor exit 0 + test updates.
8. [x] git warning convention + test if present.
9. [x] joinKeys/label-matcher dedup.
10. [x] task list --limit + tests.
11. [x] gofmt/vet/full test suite; smoke-test against the dogfood board.
12. [x] SKILL.md: --limit, dry_run key, doctor exit contract, last_id;
        rebuild + reinstall binary and skill.

## Deferred / explicitly out of scope

- Config lock + output flags (config is separate from the board by design).
- Lock ownership tokens / mtime refresh (rejected as over-complication).
- Docs pass (CLAUDE.md "freeform" status claim, SKILL.md doctor exit note,
  --labels vs --label rationale) — separate follow-up after this lands.

## Change history

- [2026-07-14] Plan created from the reading-pass findings; decisions:
  last_id in config.yml, doctor pure exit 0, staleAfter 120s.
- [2026-07-14] All ten items implemented and verified end-to-end on a scratch
  board (id-reuse closed with scaffold comments intact; last_id visible via
  config list/get and refused by set; single-mutation plain rows; empty plain
  list silent; --limit; doctor exit 0 with findings; cleanup dry_run key;
  auto-commit includes the config.yml bump in the create commit). Two legacy
  tests asserting the old behaviours (id reuse, doctor non-zero exit) were
  updated; `task state <id> --json` now carries the standard "warnings" key
  like the other mutations. Full suite green.

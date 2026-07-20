# 061 — fourth cleaning pass

## Summary

Fixes for the fourth full-source audit (post-060, commit dd658bc). Findings
F1, F2, F3 were reproduced on a scratch board; scope triaged with the user
[2026-07-19]. Guiding principle agreed for operations that meet a broken task
file: if the operation only affects parts that are still valid, continue and
warn that the file is broken (recommend `doctor --fix`); if the part being
operated on is itself broken/missing, refuse rather than guess.

- **F1 — `doctor --fix` must not remove deps that reference unparseable
  files.** The dangling-dep repair checks deps against ids of *parsed* files
  only, so a dep on a real-but-broken file is reported as "unknown task" and
  deleted — destroying a valid link the moment the target file is repaired.
  Fix: collect the filename ids of unparseable files (`filenameID`; every
  scanned file has one — `taskFileRe` requires a leading id) and treat deps
  matching them as report-only ("references task N whose file is unparseable —
  repair it first"), never removed by `--fix`. Deps to genuinely unknown ids
  are still removed.
- **F2 — cap the title slug.** An over-long title exceeds the 255-byte
  filename limit: create fails with an internal error *after* `AllocateID`
  persisted the `last_id` bump, burning an id. Fix: `board.Slug` truncates to
  80 chars (no trailing dash). Fixes both the failure and the id burn.
- **F3 — `doctor --fix` makes one commit.** Today duplicate renumbering and
  dangling-dep removal auto-commit per repair while the CRLF rewrite, drift
  rename, and `.gitattributes`/`.gitignore` restores never commit — one run
  leaves a half-committed tree. Fix: in fix mode, suppress per-repair commits,
  collect every changed/removed path across all passes, and make a single
  `north: doctor fixes` commit at the end (only when `auto_commit` is on and
  something was repaired). A final-commit failure is returned as an error.
- **F4 — `05_configuration.md` drift.** It still claims `config set` drops
  comments; the 058 node round-trip preserves them. Reword.
- **F5 — TUI board popup wraps Markdown at the popup's own width.** The
  enter-popup reuses the list pane's cached glamour renderer (wrapped at
  ~65 % width) inside a `width−14` viewport. Fix: render popup content with a
  renderer built for the popup width.
- **F6 — `delete` heals broken dependents too, when their deps are
  readable.** Healing walks the snapshot, which drops unparseable and
  duplicate-id files, so a broken dependent silently keeps its dangling ref.
  Per the agreed principle, the heal targets `depends_on` — if that part is
  readable the operation proceeds even though the file is otherwise broken.
  Fix: for every snapshot-warned file, attempt a surgical heal — re-read the
  frontmatter node (`splitFrontmatter`/`parseFront`); when it parses and
  `depends_on` contains the deleted id, remove just that entry from the node
  and rewrite the file (frontmatter + body preserved verbatim otherwise, no
  `updated_at` bump — the file stays broken), include it in the commit, and
  warn "`<file>` is broken but depended on X — removed it from depends_on;
  run `north doctor` to repair the file". When the frontmatter itself is
  unreadable, healing is impossible: warn "`<file>` could not be checked for
  dependency healing — repair it, then run `north doctor`". At hint level
  nothing is healed (unchanged); broken files with readable deps containing
  the id are named in the dangling warning.
- **F7 — document `skill install --target`.** Missing from the 03_cli.md
  table and the README.
- **F8 — `skill check` grades installs fully.** Presence of the file
  determines installed; everything else is graded in the report:
  `missing` (no file) | `unstamped` (no version stamp) | `outdated`
  (stamp ≠ binary) | `modified` (stamp matches, text differs from the
  embedded skill) | `ok`. Every non-ok human line recommends
  `north skill install`. Exit stays 0 (doctor-style); the JSON shape is
  unchanged, its `status` enum grows.
- **F9 — `task list` validates flags before touching the board.**
  `--sort`/`--deps`/`--limit` are currently validated after the snapshot
  load (`--status` after locate). Move all usage validation ahead of
  `board.LocateBoard`, matching `next`.

Declined: audit item 10 (create-time self-dep at hint level) — accepted as-is.

## Files to modify

| Finding | Files |
|---|---|
| F1 | `internal/tasks/doctor.go`, `internal/tasks/doctor_test.go` |
| F2 | `internal/board/board.go` (Slug), `internal/board/board_test.go` |
| F3 | `internal/tasks/doctor.go`, `internal/tasks/tasks.go` (save/commit split), doctor tests |
| F4 | `docs/design/05_configuration.md` |
| F5 | `internal/tui/list.go`, `internal/tui/model.go`, `internal/tui/tui_test.go` |
| F6 | `internal/tasks/tasks.go` (Delete), `internal/tasks/deps_test.go` |
| F7 | `docs/design/03_cli.md`, `README.md` |
| F8 | `internal/cli/skill.go`, `internal/cli/cli_test.go`, `docs/design/03_cli.md` |
| F9 | `internal/cli/task.go`, `internal/cli/cli_test.go` |

## Todo

1. [x] Write this plan
2. [x] Wave 1 — tasks/board core: F1, F2, F3, F6 (+tests: unparseable-ref dep
       survives `--fix` with report-only issue; 300-char title creates, slug
       ≤ 80; single doctor commit with auto_commit on; delete warning with a
       broken file present)
3. [x] Wave 2 — cli/skill: F8, F9 (+tests: unstamped/modified statuses; list
       flag errors occur without a board present)
4. [x] Wave 3 — tui: F5 (+test: popup content wrapped to popup width)
5. [x] Wave 4 — docs: F4, F7; 03_cli.md skill-check status enum + doctor
       single-commit note; 02_board-data-model.md dangling-`--fix` caveat
6. [x] Verify: `make build vet` + `go test ./...` + scratch-board probes
       (doctor unparseable-ref preservation, one-commit fix run, long title,
       skill check statuses, list flag validation order)

## Change history

- [2026-07-19] Audit completed (full source read; F1/F2/F3 reproduced on a
  scratch board); findings triaged with the user across two rounds; broken-file
  handling principle and skill-check grading agreed; plan written. No code
  changes yet.
- [2026-07-20] All nine findings implemented across four parallel waves.
  - F1/F3: `doctor.go` now collects `unparseableIDs` (deps matching them are
    report-only, never removed) and accumulates `fixedPaths`/`removedPaths`
    across every pass into one `north: doctor fixes` commit.
  - F2: `board.Slug` truncates to `slugMaxLen` (80). The regex is
    ASCII-only, so the byte slice cannot split a rune.
  - F6: `save` split into `persist` (write only) + commit; `Delete` gained
    `healBrokenDependents`, which edits only the `depends_on` node of
    snapshot-excluded files and warns when the frontmatter is unreadable.
  - F5: `newMarkdownRenderer`/`renderDetailAt` render the board popup at the
    popup's own width without disturbing the list pane's cached renderer.
  - F8: `skill check` grades missing | unstamped | outdated | modified | ok.
  - F9: `task list` validates all flags before `board.LocateBoard`.
  - F4/F7 plus the 03_cli.md status enum and 02_board-data-model.md caveats.
- [2026-07-20] Verification: `make build vet` clean, `go test ./...` all green,
  and scratch-board probes confirmed each fix end to end (300-char title →
  85-char filename with `last_id` at 1; `--sort bogus` outside a board exits 2;
  skill check reports `modified` after a hand edit; `doctor --fix` kept the dep
  on the unparseable file, dropped `99`, made exactly one commit and left the
  tree clean; `delete` healed the broken dependent while preserving its
  `status: bogus` and body).
  Two defects were found and fixed during verification, both in Wave 1's tests
  rather than its product code: missing `os`/`filepath` imports in
  `deps_test.go` (the sub-agent was cut off mid-file by a session limit), and
  an unsound assertion in `TestDoctorFixSingleCommit` that expected the
  CRLF-repaired file in the doctor commit. Because `.gitattributes` sets
  `* text eol=lf`, git normalises the file at `add` time, so the setup commit
  already stores it LF and doctor's rewrite yields an identical blob — git is
  right to omit it. The test now asserts on a dangling-dep repair (a genuine
  content change from a different pass) and checks the CRLF file is LF on disk,
  with the clean-tree assertion covering that repair.

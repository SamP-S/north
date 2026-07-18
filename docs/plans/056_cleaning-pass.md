# Codebase cleaning pass — findings & fix plan

## Context
User requested a read-only cleaning pass over all code/tests to flag issues. Three parallel audits covered (1) internal/tasks + internal/board, (2) internal/cli + internal/tui, (3) cmd/north + render/git/skill/models/errors/version + build files. Top findings were independently re-verified against source. This plan lists the findings and, if approved, the fixes to apply. On approval, copy this plan to `docs/plans/056_cleaning-pass.md` per project convention.

## Findings (ranked)

### Correctness
1. **doctor --fix misses duplicates on drifted files** — `internal/tasks/doctor.go:120,130,152`. Pass 2 renames id-drifted files in place, but `byID` keeps the pre-rename path; the duplicate-id repair later calls `loadTask(path)` on the stale path, fails silently, and leaves the duplicate unfixed (needs a second `--fix` run). Fix: update `byID`/`tasksParsed` paths after a successful rename (or rename after grouping).
2. **`next --limit N --plain` prints a stray blank line on empty result** — `internal/cli/next.go:123-131`. Empty guard is `len(picked)==0 && !plain`; plain+empty falls through to `cmd.Println("")`. Contradicts the documented "empty output under --plain" contract and `task list --plain` behavior (task.go:352 guards `if out != ""`). Fix: guard plain output the same way; add empty-board `--plain` test.
3. **`dedupIDs` mutates the caller's slice** — `internal/tasks/tasks.go:217` (`out := ids[:0]`). Callers in `Create`/`Edit` pass user-supplied slices whose backing arrays get rewritten. Latent aliasing bug. Fix: allocate a fresh slice.
4. **`git.CommitBoard` treats any `diff --cached` failure as "has changes"** — `internal/git/git.go:44`. A genuine git error (exit 128) is swallowed and falls through to `commit` instead of surfacing. Fix: distinguish exit code 1 (changes) from other errors.
5. **`splitFrontmatter` closing-fence match is too loose** — `internal/tasks/frontmatter.go:51`. `strings.Index(rest, "\n---")` matches `\n----` or a meta line starting with `---`, truncating frontmatter early. Fix: require the fence to be exactly `---` on its own line (`\n---\n` or trailing `\n---` at EOF).
6. **Missing-`version` default will misclassify legacy boards when the format bumps** — `internal/board/board.go:254,303`. Absent `version:` defaults to `FormatVersion` (currently 1, so harmless; wrong the day it becomes 2). Fix: default to literal 1; dedupe the parse logic shared by `LoadConfig`/`checkVersion`.
7. **`SetStatus` no-op early-return skips the assigned-ready warning** — `internal/tasks/tasks.go:461`. Re-issuing `ready` on an already-ready assigned task returns no warning. Minor; fix or document.

### Duplication / dead code
8. **Search matcher duplicated 3×** — `internal/cli/task.go:373` (`filterSearch`), `internal/tui/list.go:369` (`matchesFilter`), `internal/tui/deps.go:98` (`matchEntry`). Identical 5-field substring match; comments demand lockstep. Fix: one shared helper (e.g. `tasks.MatchesSearch(task, query)`).
9. **Unreachable branch in `depsPolicy`** — `internal/tasks/tasks.go:209-211`. `cfg.DepsEnforcement == ""` can never happen (DefaultConfig seeds it; ParseDepsEnforcement rejects empty). Remove.
10. **`warnDependents` return value unused** — `internal/cli/task.go:640,664`. Drop the return, or use it.
11. **`skill.Agents()` has no non-test caller** — `internal/skill/install.go:26`. Dead export; remove or justify.

### Docs-in-code drift
12. **Stale comments naming removed behavior/keys** — `internal/cli/doctor.go:36` ("Gate on the --json issues array" — doctor deliberately exits 0); `internal/tui/modal.go:31-32` (says keys `D`/`L`; actual bindings are `x`/`w` per keys.go:88-99). Fix comments.
13. **Test fixture uses nonexistent frontmatter key** — `internal/tasks/guards_test.go:77` emits `agent: ""` but the real key is `assignee`. Fix fixture.

### Styling (TUI)
14. **State/sort pickers render through `statusStyle`** — `internal/tui/modal.go:264-269`. "draft/active/archive" and sort labels fall through to unstyled default, unlike everywhere else that uses `stateStyle`. Colors-only fix (no glyph changes, per theme invariant).

### Test gaps / hygiene
15. **git tests non-hermetic** — `internal/git/git_test.go:25`. `initRepo` doesn't isolate `GIT_CONFIG_GLOBAL`/`GIT_CONFIG_SYSTEM` (only `TestCommitBoard_NoIdentity` does); host gpgsign/hooksPath breaks 5 tests. Fix: move the env isolation into `initRepo`/`runGit`.
16. **No test pins the theme "colors-only, chrome never changes" invariant** — a preset swapping border glyphs would pass. Add a test asserting all presets share identical border/bold/padding structure.
17. **`CommitBoard` pathspec-scoping guarantee untested** — no test asserts an unrelated staged file survives the commit.
18. **Missing per-identifier doc comments on exported constants** — `internal/models/models.go:21-25,40-46` (`StateDraft…`, `Ready…Blocked`).
19. *(note only)* `render.go:34` local `tasks := taskList` shadows the `tasks` package name — readability footgun, rename if touched.

## Decisions (user-confirmed 2026-07-15)
- **#5 fence match**: closing fence = a line that is exactly `---` plus optional trailing whitespace (`\n---[ \t]*(\n|EOF)`); `----`/`--- text` no longer terminate frontmatter.
- **#7 no-op warning**: warn even on no-op — move the assigned-while-ready check ahead of the `target == task.Status` early return in `SetStatus`.
- **#11 skill.Agents()**: remove the export; retarget its test at the unexported `agents` data.
- **#6 version default**: absent `version:` key defaults to literal `1` (not `FormatVersion`); dedupe the parse/compare logic shared by `LoadConfig` and `checkVersion`. No stamping-on-write change.

## Fix plan (if approved)
Files to modify: `internal/tasks/{doctor.go,tasks.go,frontmatter.go}`, `internal/board/board.go`, `internal/cli/{next.go,task.go,doctor.go}`, `internal/tui/{modal.go}`, `internal/git/{git.go,git_test.go}`, `internal/skill/install.go`, `internal/models/models.go`, `internal/tasks/guards_test.go`, plus new/extended tests in `internal/cli/next_test.go`, `internal/tui/tui_test.go`, `internal/git/git_test.go`, `internal/tasks/doctor_test.go`.

Todo:
1. [x] Fix doctor drift+duplicate interaction (#1) + regression test
2. [x] Fix empty `next --plain` (#2) + test
3. [x] Fix `dedupIDs` aliasing (#3)
4. [x] Fix `CommitBoard` diff error handling (#4)
5. [x] Tighten `splitFrontmatter` fence match (#5) + edge tests
6. [x] Version default literal 1 + dedupe (#6)
7. [x] Shared search matcher (#8); remove dead code (#9–#11)
8. [x] Comment/fixture fixes (#12, #13); doc comments (#18)
9. [x] Picker stateStyle fix (#14)
10. [x] Test hygiene: hermetic git tests (#15), theme invariant test (#16), pathspec test (#17)
11. [x] `make fmt vet test` green; copy plan to docs/plans/056

Change history: [2026-07-15] plan created from audit findings. [2026-07-15] all 11 todo items implemented via three parallel sub-agents; full suite green (go test -count=1 ./...), e2e-verified empty `next --plain` and single-run doctor --fix on a drifted duplicate. [2026-07-15] follow-up: release workflow now cross-compiles via new `make dist` (linux/darwin amd64+arm64, windows amd64) and attaches the stamped binaries to GitHub releases, closing the audit's ldflags-untested-in-CI note. [2026-07-16] second-pass fixes landed: doctor no-clobber rename, numeric id validation, cleanup negative rejection, wrapped mutation JSON, SHA256SUMS + windows/arm64 + clean dist, fence/picker regression tests; e2e-verified no-clobber heal and traversal rejection.

## Second pass (2026-07-15, post-4eceac7 audit)

Findings (user-approved for fix; macOS CI matrix item explicitly skipped):

- **S1 doctor --fix rename clobber (data loss)** — `internal/tasks/doctor.go:129`: id-drift repair `os.Rename` silently overwrites an existing target when a drifted duplicate shares the legit task's title slug (`5-foo.md` + `9-foo.md` id 5). Fix: refuse the rename when the target exists (leave issue unfixed for the duplicate pass to renumber first, or guard + report); add colliding-slug regression test.
- **S2 path traversal via frontmatter id** — ids are interpolated raw into `TaskFilename` (`internal/board/board.go:469`) and never validated on load. Fix: `loadTask` rejects ids not matching `^[0-9]+$` as Invalid (doctor then surfaces such files as unparseable).
- **S3 release checksums** — `make dist` now also writes `dist/SHA256SUMS`.
- **S5 cleanup negative validation** — `cleanup --older-than` with negative value silently means "no age filter"; reject negatives with Invalid like `--limit` flags do (`internal/cli/board.go`).
- **S6 JSON shape unification** — mutation commands (`task create/edit/move/state/delete`, batch ops) flatten task fields top-level with injected `warnings`; `next`/`take` wrap as `{"task":…,"warnings":[…]}`. DECIDED: unify on the wrapped shape everywhere a single task is returned; update embedded SKILL.md and tests. (Pre-1.0 breaking change, accepted.)
- **S7 Makefile polish** — `clean` removes `dist/`; add `windows/arm64` to `DIST_PLATFORMS`.
- **S8 test gaps** — pin fence edges (BOM, empty meta, fence at EOF, body starting with `---`) and the state-picker `stateStyle` rendering.

Second-pass todo:
1. [x] S1 rename guard + colliding-slug test
2. [x] S2 id validation in loadTask + test
3. [x] S8a fence edge tests
4. [x] S5 negative --older-than rejection + test
5. [x] S6 JSON unification + SKILL.md + tests
6. [x] S8b picker style regression test
7. [x] S3 SHA256SUMS; S7 clean/dist + windows/arm64
8. [x] Full suite green; commit

## Verification
`make fmt && make vet && make test`; manually exercise `north next -l 2 --plain` on an empty board and `north doctor --fix` on a board with a drifted duplicate.

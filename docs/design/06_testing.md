# 6. Testing

Tests run against a real board scaffolded in a `t.TempDir()` (the `newBoard`
helper in each package's `_test.go`). No mocking of the filesystem — the core
is plain file I/O, so tests exercise the real thing. Git tests execute the
real `git` binary in temp repos (including a linked-worktree case and a
no-identity case isolated via `GIT_CONFIG_GLOBAL=/dev/null`).

| File | Covers |
|---|---|
| `internal/board/board_test.go` | discovery (walk-up), `init` scaffolding (incl. template/.gitattributes), id allocation, strict config parsing, format-version stamp/refusal |
| `internal/board/lock_test.go` | advisory lock acquire/release, retry while held, stale-lock steal, held-lock conflict |
| `internal/tasks/tasks_test.go` | create/list/edit/delete, freeform status, filename slugs, append-body |
| `internal/tasks/archive_test.go` | freeform state moves, status preservation, cleanup, state↔status orthogonality |
| `internal/tasks/parse_test.go` | tolerant loading (bad files → warnings), CRLF, scalar coercion, unknown-key round-trip, duplicate-id warnings, editor-doc round-trip |
| `internal/tasks/doctor_test.go` | doctor detection (duplicates, CRLF, unparseable incl. unknown statuses, dangling deps, cycles, drift, missing .gitattributes) and `--fix` repairs |
| `internal/tasks/guards_test.go` | not-found ops, cleanup `--older-than`, id reservation |
| `internal/tasks/deps_test.go` | depends_on validation, Dependents scanning |
| `internal/tasks/sort_test.go` | sort keys (id/updated/title/assignee), directions, unassigned-last |
| `internal/tasks/autocommit_test.go` | `auto_commit` commits locally / is off by default |
| `internal/git/git_test.go` | exec-git staging/commits, subdir boards, linked worktrees, identity fallback |
| `internal/render/render_test.go` | `--plain` / `--json` output shape (incl. warnings arrays) |
| `internal/cli/cli_test.go` | CLI dispatch, state/move flow, batch ids (move/state/delete, continue-on-error), delete `-y` contract, search/label/assignee filters, plain columns, template-filled creates, exit-code contract, config cmd (incl. read-only version), doctor, JSON errors, init epilogue/modes, nested/newer-board refusal, skill check |
| `internal/skill/skill_test.go` | embedded skill content, version stamps, install targets |
| `internal/tui/tui_test.go` | shared modals in both views, state picker apply, doctor popup (report + `f` fix/reload), q/help/r behaviour, display-width truncation, filter-clearing selection, editor templates |

Gate before merging:

```bash
make vet     # go vet + gofmt check
make test    # go test ./...
```

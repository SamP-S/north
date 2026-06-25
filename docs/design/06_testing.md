# 6. Testing

Tests run against a real board scaffolded in a `t.TempDir()` (the `newBoard`
helper in each package's `_test.go`). No mocking of the filesystem — the core is
plain file I/O, so tests exercise the real thing.

| File | Covers |
|---|---|
| `internal/board/board_test.go` | discovery (walk-up), `init` scaffolding, id allocation |
| `internal/tasks/tasks_test.go` | create/read/list/edit/move/delete, the transition table, filename slugs |
| `internal/tasks/archive_test.go` | archive + cleanup, archive↔status orthogonality |
| `internal/tasks/autocommit_test.go` | `auto_commit` commits locally / is off by default |
| `internal/render/render_test.go` | `--plain` / `--json` output shape |
| `internal/cli/cli_test.go` | CLI dispatch, `--json` output, error exit codes |

Gate before merging:

```bash
make vet     # go vet + gofmt check
make test    # go test ./...
```

# 6. Testing

Tests run against a real board scaffolded in a `t.TempDir()` (the `newBoard`
helper in each package's `_test.go`). No mocking of the filesystem — the core
is plain file I/O, so tests exercise the real thing. Git tests execute the
real `git` binary in temp repos (including a linked-worktree case and a
no-identity case isolated via `GIT_CONFIG_GLOBAL=/dev/null`). TUI tests are
behavioural (drive the bubbletea model), not snapshot-based — golden-file
maintenance was rejected.

Coverage lives next to the code it covers: each `internal/<pkg>` owns its
tests, `internal/cli/cli_test.go` exercises the full command surface and the
output/exit-code contract end-to-end. The test files are the inventory —
this doc doesn't duplicate them.

Gate before merging:

```bash
make vet     # go vet + gofmt check
make test    # go test ./...
```

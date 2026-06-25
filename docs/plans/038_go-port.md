# Plan 038: Port North to Go

## Context

North is currently a Python CLI + optional MCP server (~1050 lines). The goal is a total **in-place rewrite** in Go: the Go sources replace the Python ones in this same repo, producing a single compiled binary with no runtime dependency. The Python package, `pyproject.toml`, `uv.lock`, `tests/`, and `scripts/install*.sh` are removed (recoverable via git history); `docs/` and `CLAUDE.md`/`README.md` are updated. The deleted Python tree remains the behavioural reference (read from git history during the port).

### Decisions locked with the user
1. **In-place rewrite** — no parallel/`python/` directory; Python sources deleted entirely.
2. **Standard Go layout** — `cmd/north` is the only `main` package (the only installable binary); *all* library code lives under `internal/` so nothing else is importable externally.
3. **Module path `github.com/SamP-S/north`** — git-style path so `go install github.com/SamP-S/north/cmd/north@latest` works. Internal imports still resolve locally on disk; the prefix is just the module identity.
4. **Task-file format** — keep the same frontmatter schema (`id, title, status, agent, labels, depends_on, created_at, updated_at` + body); emit valid YAML. Byte-for-byte match with PyYAML output is **not** required. Timestamps are full ISO-8601 datetimes with timezone (matching the actual Python code, e.g. `2026-06-25T00:08:00Z`, not the date-only form shown in plan 037's schema example).

---

## Target repo layout (standard Go)

```
<repo root>
  cmd/north/main.go        <- the only main package; calls internal/cli.Execute()
  internal/
    errors/errors.go       <- BoardError, NotFound, Conflict, Invalid
    models/models.go       <- TaskStatus, Task struct, TRANSITIONS map, STATUS_DIRS
    board/board.go         <- LocateBoard, InitBoard, LoadConfig, WriteConfig, NextID, TaskFiles, Slug, TaskFilename
    tasks/tasks.go         <- Create, Get, List, Edit, Move, Archive, Cleanup, Delete, StatusCounts (+ frontmatter read/write)
    git/git.go             <- CommitBoard (go-git; no-op if not in a git repo)
    instructions/instructions.go  <- AgentsMD() string (same text as Python source)
    render/render.go       <- RenderTaskList, RenderTask (human / --plain / --json)
    cli/
      root.go              <- cobra root command, Execute(), error handling
      init.go              <- `north init`
      task.go              <- `north task {create,view,list,edit,move,archive,delete}`
      board.go             <- `north board`
      cleanup.go           <- `north cleanup [--older-than DAYS]`
      mcp.go               <- `north mcp {start,stop,status,run}`
    service/
      server.go            <- net/http server: /mcp (mcp-go handler) + GET /health
      config.go            <- load MCP_TOKEN via godotenv + os.Getenv
      mcp.go               <- build mcp-go server, register 5 tools
  go.mod
  go.sum
  README.md  CLAUDE.md  docs/  .gitignore
```

**Removed:** `north/` (Python package), `pyproject.toml`, `uv.lock`, `tests/` (Python), `scripts/install*.sh`, `.venv/`.

---

## External dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | Subcommand CLI (replaces argparse) |
| `gopkg.in/yaml.v3` | `config.yml` read/write + frontmatter YAML block |
| `github.com/go-git/go-git/v5` | `auto_commit` — stage + commit changed files |
| `github.com/mark3labs/mcp-go` | MCP streamable-HTTP server + tool registration |
| `github.com/joho/godotenv` | Load `MCP_TOKEN` from `.env` file |

Frontmatter parsing is hand-rolled (~20 lines: split on `---` delimiters, parse YAML block with yaml.v3). No separate frontmatter library needed.

---

## Key design notes

**Frontmatter**: Split file content on `---\n` boundaries, unmarshal middle block with yaml.v3, treat remainder as body. On write, marshal meta back to YAML, wrap in `---\n...\n---\n\n<body>\n`.

**MCP server process management**: The binary is its own MCP server. `north mcp run` starts `net/http` in the foreground. `north mcp start` execs `north mcp run` as a detached subprocess (same pattern as Python: PID file at `~/.north/mcp.pid`, log at `~/.north/mcp.log`). `north mcp stop` sends SIGTERM. `north mcp status` reads the PID file.

**Board discovery**: Walk up from `os.Getwd()` looking for `north/config.yml` — same logic as Python's `locate_board()`.

**Error handling**: `BoardError` interface with a `Code()` string method; `NotFound`, `Conflict`, `Invalid` implement it. CLI root catches `BoardError` + `CLIError` and prints to stderr with exit 1.

**`--plain` / `--json`**: Persistent flags on the task subcommand group; passed down to `render.RenderTaskList` / `render.RenderTask`.

**No `.env` auto-load at init**: `godotenv.Load()` is called at server startup only (same as Python behaviour — dotenv is not loaded for pure CLI operations).

---

## Ordered todo

- [x] 1. `go mod init github.com/SamP-S/north`; add deps; scaffold `cmd/` + `internal/` tree; `.gitignore` for Go
- [x] 2. `internal/errors` — BoardError interface + NotFound/Conflict/Invalid types
- [x] 3. `internal/models` — TaskStatus, Task struct, STATUS_DIRS, TRANSITIONS
- [x] 4. `internal/board` — LocateBoard, InitBoard, LoadConfig, WriteConfig, NextID, TaskFiles, Slug, TaskFilename
- [x] 5. `internal/tasks` — full CRUD: Create, Get, List, Edit, Move, Archive, Cleanup, Delete, StatusCounts (+ hand-rolled frontmatter read/write)
- [x] 6. `internal/git` — CommitBoard via go-git (best-effort, no-op outside git repo)
- [x] 7. `internal/instructions` — AgentsMD() returning the AGENTS.md text
- [x] 8. `internal/render` — RenderTaskList, RenderTask (human/plain/json)
- [x] 9. `internal/cli` — all cobra commands: root + init, task (create/view/list/edit/move/archive/delete), board, cleanup, mcp (start/stop/status/run)
- [x] 10. `cmd/north/main.go` — call `cli.Execute()`; wire exit codes / error unwrapping
- [x] 11. `internal/service/config.go` — godotenv load + MCP_TOKEN from env
- [x] 12. `internal/service/mcp.go` — register 5 mcp-go tools (list_tasks, get_task, create_task, set_task_status, edit_task)
- [x] 13. `internal/service/server.go` — net/http mux: mount mcp-go handler at `/mcp`, GET `/health`
- [x] 14. Tests — 32 Go tests using `testing` + `t.TempDir()` tmp boards; one `_test.go` per internal package + CLI command tests
- [x] 15. `go build ./cmd/north` — clean compile; `go test ./...`; `go vet ./...`; e2e smoke test
- [x] 16. **Remove Python**: `git rm -r north/ tests/ pyproject.toml uv.lock scripts/install*.sh`; updated `README.md` + `CLAUDE.md` + `docs/design/`; added a Go `Makefile`

---

## Verification

1. `go build ./cmd/north` produces a single binary, no errors
2. `go test ./...` — all tests pass
3. E2e in a tmp git repo:
   - `north init` → `north/config.yml` + 6 status dirs + `archive/` + `AGENTS.md`
   - `north task create "Add login" --agent opus4.8` → lands in `draft/`
   - `north task move task-1 ready` → `north task move task-1 in_progress` → `north task move task-1 done`
   - `north board` → correct counts
   - `north task list --json` → valid JSON
   - From a subdirectory: `north task list` finds board by walking up
   - `auto_commit: true` in config → each mutation produces a local git commit
   - `north mcp start` → `curl http://127.0.0.1:8001/health` returns 200 → `north mcp stop`

## Change history

- [2026-06-25] Drafted (separate repo).
- [2026-06-25] Reworked to an **in-place rewrite**: standard Go layout with all library code under `internal/` (only `cmd/north` installable); module `github.com/SamP-S/north`; Python sources removed via git; task-file schema kept, valid-YAML (not byte-identical) output, full ISO-8601 timestamps.
- [2026-06-25] Implemented end-to-end on branch `go-port`. Built `internal/{errors,models,board,tasks,git,instructions,render,cli,service}` + `cmd/north`; deps cobra, yaml.v3, go-git/v5, mcp-go, godotenv. Removed the Python package, `tests/`, `pyproject.toml`, `uv.lock`, `scripts/install*.sh`; rewrote `.gitignore`, README, CLAUDE.md, `docs/design/{04_mcp,06_testing}`; added a `Makefile`. Verified: `go build ./...` + `go vet ./...` clean, `gofmt` clean, **32 tests pass**; manual e2e (init → create → move chain → board → list --json/--plain), discovery from a subdir, illegal-transition / unknown-status / not-found errors, `auto_commit` local commits (incl. file moves), and the MCP server (`mcp start/status/stop`, health 200, `tools/list` = 5 tools, `tools/call list_tasks` returns data). Deviations: YAML lists render block-style (4-space indent) vs PyYAML's flow `[...]` — valid YAML, round-trips, as agreed.
- [2026-06-25] Removed the `north instructions` CLI command (not relevant to this build): dropped `newInstructionsCmd` + its registration and test, updated `README` + `docs/design/03_cli.md`. `AGENTS.md` is still written by `north init`, so the `internal/instructions` package stays.

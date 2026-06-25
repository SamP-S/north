# Plan 038: Port North to Go

## Context

North is currently a Python CLI + optional MCP server (~1050 lines). The goal is a total rewrite in Go to produce a single compiled binary with no runtime dependency. The Python source in this repo is the reference implementation. This plan covers a new standalone repo; the Python repo is kept as-is for reference.

---

## Target repo layout

```
north/                     <- new repo root
  cmd/north/main.go        <- entry point, calls cli.Execute()
  internal/
    errors/errors.go       <- BoardError, NotFound, Conflict, Invalid
    models/models.go       <- TaskStatus (iota), Task struct, TRANSITIONS map, STATUS_DIRS
    board/board.go         <- LocateBoard, InitBoard, LoadConfig, WriteConfig, NextID, TaskFiles, Slug, TaskFilename
    tasks/tasks.go         <- Create, Get, List, Edit, Move, Archive, Cleanup, Delete, StatusCounts
    git/git.go             <- CommitBoard (go-git; no-op if not in a git repo)
    instructions/instructions.go  <- AgentsMD() string (same text as Python source)
  cli/
    root.go                <- cobra root command, error handling
    commands/
      init.go              <- `north init`
      task.go              <- `north task {create,view,list,edit,move,archive,delete}`
      board.go             <- `north board`
      cleanup.go           <- `north cleanup [--older-than DAYS]`
      instructions.go      <- `north instructions`
      mcp.go               <- `north mcp {start,stop,status,run}`
    render/render.go       <- RenderTaskList, RenderTask (human / --plain / --json)
  service/
    server.go              <- net/http server: POST /mcp (mcp-go handler) + GET /health
    config.go              <- load MCP_TOKEN via godotenv + os.Getenv
    mcp.go                 <- build mcp-go server, register 5 tools
  go.mod
  go.sum
```

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

- [ ] 1. Init repo: `go mod init github.com/SamP-S/north`, add all deps, scaffold directory tree
- [ ] 2. `internal/errors` — BoardError interface + NotFound/Conflict/Invalid types
- [ ] 3. `internal/models` — TaskStatus, Task struct, STATUS_DIRS, TRANSITIONS
- [ ] 4. `internal/board` — LocateBoard, InitBoard, LoadConfig, WriteConfig, NextID, TaskFiles, Slug, TaskFilename
- [ ] 5. `internal/tasks` — full CRUD: Create, Get, List, Edit, Move, Archive, Cleanup, Delete, StatusCounts (+ hand-rolled frontmatter read/write)
- [ ] 6. `internal/git` — CommitBoard via go-git (best-effort, no-op outside git repo)
- [ ] 7. `internal/instructions` — AgentsMD() returning the AGENTS.md text
- [ ] 8. `cli/render` — RenderTaskList, RenderTask (human/plain/json)
- [ ] 9. `cli/commands` — all cobra commands: init, task (create/view/list/edit/move/archive/delete), board, cleanup, instructions, mcp (start/stop/status/run)
- [ ] 10. `cli/root.go` + `cmd/north/main.go` — wire cobra root, error unwrapping, exit codes
- [ ] 11. `service/config.go` — godotenv load + MCP_TOKEN from env
- [ ] 12. `service/mcp.go` — register 5 mcp-go tools (list_tasks, get_task, create_task, set_task_status, edit_task)
- [ ] 13. `service/server.go` — net/http mux: mount mcp-go handler at `/mcp`, GET `/health`
- [ ] 14. Tests — mirror the 32 Python tests using `testing` + `t.TempDir()` tmp boards; one `_test.go` per internal package + CLI command tests
- [ ] 15. `go build ./cmd/north` — verify clean compile; `go test ./...`; e2e smoke test

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

- [2026-06-25] Drafted

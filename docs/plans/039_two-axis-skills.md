# Plan 039: Two-axis task model + drop MCP for CLI-skills

## Context

North currently conflates two concerns into one folder-based status (`draft/ ready/ in_progress/ done/ failed/ blocked/` + a separate `archive/`), and ships an optional MCP server for AI integration. Two changes:

1. **Split into two orthogonal axes** (the model backlog.md uses):
   - **State** = lifecycle *location* (the folder): `draft` → `active` → `archive`.
   - **Status** = workflow *column* (frontmatter only): `ready, in_progress, done, failed, blocked`.
   This fixes the design wart where archive already had to read status from frontmatter while everything else read it from the folder. Status lives in frontmatter for *every* task; state is the folder.

2. **Drop the MCP server**; do AI integration via **CLI + an installable skill file** (the kanban-md / backlog.md direction). `north init` stops writing `AGENTS.md`; instead a single embedded `SKILL.md` is installed into the user's agent dirs via `north skill install`.

Both are **breaking** changes; the tool is pre-release with no real boards, so no migration (decided with user). Work continues on branch `go-port` (nothing committed yet).

### Decisions locked with user
- `create` lands in **drafts/** (keep the human gate); `promote` activates it.
- Reverse state moves: **both** `restore` (archive→active) and `demote` (active→draft).
- `north skill install` defaults to **project** dirs; `--global` opt-in.
- **Breaking, no migration.**

---

## New model (`internal/models`)

```go
type TaskState string   // the folder a task lives in
const ( StateDraft="draft"; StateActive="active"; StateArchive="archive" )
var StateDirs = map[TaskState]string{ StateDraft:"drafts", StateActive:"tasks", StateArchive:"archive" }

type TaskStatus string  // frontmatter workflow column (draft REMOVED)
const ( Ready; InProgress; Done; Failed; Blocked )
var Statuses = []TaskStatus{Ready, InProgress, Done, Failed, Blocked}
```

- **Status transitions (active-only):** `ready→in_progress`, `in_progress→{done,failed,blocked}`, `{done,failed,blocked}→ready`. Rejected with `Conflict` when the task isn't `active`.
- **State transitions:** `draft→active` (promote), `active→draft` (demote), `draft→archive` & `active→archive` (archive), `archive→active` (restore).
- **Orthogonality rules:** every task has a frontmatter `status` (default `ready`). State moves **preserve** status and only move the file between folders. Status changes rewrite frontmatter **in place** (no file move). Title changes rename the file within its current folder. `Task.Archived bool` is replaced by `Task.State`.

### Board layout (breaking)
```
north/
  config.yml          # slimmed: auto_commit only (mcp_port removed)
  drafts/             # state = draft
  tasks/              # state = active   (status in frontmatter)
  archive/            # state = archive
```

---

## Suggested CLI surface (confirm exact names at implementation)

**Lifecycle / state**
| Command | Action |
|---|---|
| `north task create <title> [--agent --labels --depends-on --body\|--body-file]` | new task in `drafts/`, status `ready` |
| `north task promote <id>` | draft → active |
| `north task demote <id>` | active → draft |
| `north task archive <id>` | draft/active → archive |
| `north task restore <id>` | archive → active |
| `north task delete <id> [-y]` | remove file |

**Status (active-only)**
| `north task move <id> <status>` | set status (`ready\|in_progress\|done\|failed\|blocked`); rejects non-active |

**Query**
| `north task view <id> [--plain\|--json]` | one task (state + status + body) |
| `north task list [--state draft\|active\|archive] [--status S] [--plain\|--json]` | default `--state active` |
| `north board` | status counts for active tasks + a drafts/archive tally |
| `north cleanup [--older-than DAYS]` | archive active `done` tasks |

**Skill**
| `north skill install [--global]` | write embedded `SKILL.md` into agent dirs |
| `north skill show` | print the embedded skill |

Inspiration: backlog.md uses draft→promote + `task archive`; kanban-md uses `move ID status` for status and `archive`. North keeps `move` for *status* and dedicated verbs for *state*.

---

## File-by-file changes

**`internal/models/models.go`** — add `TaskState` + `StateDirs`; drop `Draft` from `TaskStatus`; add `Statuses`, status `Transitions` (5-state), `StateTransitions`; `Task` gains `State`, drops `Archived`; `ToMap` emits `state` + `status`.

**`internal/board/board.go`** — replace status-folder list with `drafts/tasks/archive`; `InitBoard` scaffolds those three (no `AGENTS.md`); `Config` drops `MCPPort` (just `AutoCommit`); `TaskFiles(board, states...)`; add `StateDir(state)`; `NextID` scans all three folders.

**`internal/tasks/tasks.go`** — `loadTask`: state from folder, status always from frontmatter; `Create` → draft+ready; rename `Move`→`SetStatus` (active-only, rewrite-in-place, no folder move); add `Promote`/`Demote`/`Archive`/`Restore` (validate `StateTransitions`, move file, preserve status, bump `updated_at`); `Cleanup` archives active done; `StatusCounts` counts active; `List(state, status)`.

**`internal/render/render.go`** — show `state` + `status`; list columns include state when not filtered.

**`internal/cli/`** — `task.go`: add promote/demote/restore, make `move` status-only, `list --state`; `board.go`: board+cleanup unchanged logic; **delete `mcp.go`**; `root.go`: drop `newMCPCmd`, add `newSkillCmd`; new **`skill.go`** (`install`/`show`).

**`internal/skill/`** (new, modeled on kanban-md `internal/skill`) — `embed.go` (`//go:embed skill/SKILL.md`), `registry.go` (agents: `claude`→`.claude/skills`, `opencode`→`.opencode/skill`; project + global paths; **verify opencode's path at impl**), `install.go` (write `SKILL.md` to `<target>/north/SKILL.md`, inject a `<!-- north-skill-version: X -->` comment), `skill/SKILL.md` (single file: frontmatter `name/description/allowed-tools: Bash(north *)` + CLI reference, rules, the two-axis lifecycle, `--plain`/`--json` guidance — adapted from the old `instructions` text).

**Deletions** — `internal/service/` (whole package), `internal/cli/mcp.go`, `internal/instructions/` (content folds into `SKILL.md`); `go.mod` drops `mark3labs/mcp-go` + `joho/godotenv` (→ `go mod tidy`); remove `~/.north` usage.

**Tests** — update `board`/`tasks`/`render`/`cli` tests for the new model; add state-transition tests (promote/demote/archive/restore + active-only status enforcement); add `internal/skill` install test (writes to `t.TempDir()`, asserts `SKILL.md` + version comment); drop MCP expectations.

**Docs** — README (board layout, CLI table, new skill section, remove MCP); `docs/design/`: rewrite `02_board-data-model` (two axes), `03_cli`, replace `04_mcp.md` → `04_skills.md`, slim `05_configuration` (drop `mcp_port`), update `06_testing` + `00_index`; `CLAUDE.md` (drop MCP/service, note `board`+`tasks` + `skill`).

---

## Ordered todo
- [x] 1. `internal/models`: TaskState, StateDirs, slimmed TaskStatus, Transitions + StateTransitions, Task.State
- [x] 2. `internal/board`: drafts/tasks/archive scaffolding, StateDir, TaskFiles(states…), Config drop MCPPort, no AGENTS.md
- [x] 3. `internal/tasks`: state/status split — Create(draft), SetStatus(active-only), Promote/Demote/Archive/Restore, Cleanup, StatusCounts, List(state,status)
- [x] 4. `internal/render`: state + status output
- [x] 5. `internal/skill`: embed + registry (claude/opencode) + install; author `SKILL.md`
- [x] 6. `internal/cli`: state subcommands, `move`=status, `list --state`, new `skill` cmd; delete `mcp.go`
- [x] 7. Delete `internal/service`, `internal/instructions`; `root.go` wiring; `go mod tidy`
- [x] 8. Update + add tests (model, transitions, skill install)
- [x] 9. `go build ./... && go vet ./... && go test ./...`; gofmt
- [x] 10. Docs: README, `docs/design/*`, CLAUDE.md
- [x] 11. e2e smoke (below); command names per the suggested surface (no changes requested)

---

## Verification
1. `go build ./...`, `go vet ./...`, `go test ./...` clean; `gofmt -l` empty.
2. e2e in a tmp git repo:
   - `north init` → `config.yml` (auto_commit only) + `drafts/ tasks/ archive/`, **no `AGENTS.md`**
   - `north task create "Add login"` → lands `drafts/`, status `ready`
   - `north task move task-1 in_progress` → **rejected** (not active)
   - `north task promote task-1` → file moves to `tasks/`, status still `ready`
   - `north task move task-1 in_progress` → ok (frontmatter rewrite, file stays in `tasks/`)
   - `north task move task-1 done` → ok; `north board` counts it under active/done
   - `north task archive task-1` → `archive/`, status `done` preserved; `north task restore task-1` → back to `tasks/`
   - `north task demote task-1` → back to `drafts/`
   - `north task list --state draft|active|archive`, `--json` valid
   - discovery from a subdir; `auto_commit: true` → one local commit per mutation
3. `north skill install` (project) → `./.claude/skills/north/SKILL.md` + `./.opencode/skill/north/SKILL.md` exist with version comment; `--global` → home-dir equivalents; `north skill show` prints it.
4. Grep gate: no `mcp`, `mark3labs`, `godotenv`, `~/.north`, `AGENTS.md` writes in active code.

## Change history
- [2026-06-25] Drafted: two-axis state/status split + drop MCP for CLI-installable skill. Decisions: create→drafts (gate), restore+demote both, skill install project-default/--global, breaking no-migration.
- [2026-06-26] Implemented on branch `go-port`. `internal/models` two axes (TaskState/StateDirs/StateTransitions, 5-status Transitions); `internal/board` drafts/tasks/archive scaffolding, slim Config (auto_commit only), no AGENTS.md; `internal/tasks` Create(draft)/SetStatus(active-only, in-place)/Promote/Demote/Archive/Restore/Cleanup/List(states,status); `internal/render` state+status; new `internal/skill` (embed SKILL.md + claude/opencode registry + install, version comment); CLI gained promote/demote/restore + `move`=status + `list --state` + `skill install|show`; deleted `internal/service`, `internal/cli/mcp.go`, `internal/instructions`; dropped `mark3labs/mcp-go` + `joho/godotenv` (go.mod now cobra/yaml.v3/go-git only). Verified opencode skill path = `.opencode/skills/` project, `~/.config/opencode/skills/` global (also writes `.claude/skills/`). All docs updated. `go build`/`go vet`/`gofmt` clean, 33 tests pass, full e2e (create→promote→move→board→archive→restore→demote, draft status-change rejected, skill install to both agents, auto_commit) green.

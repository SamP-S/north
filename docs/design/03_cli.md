# 3. CLI

`north <command>`. Every command (except `init`) finds the board by walking up
from the current directory. Failures print `error [<code>]: <message>` to
stderr; when the failing command was invoked with `--json`, the error is
emitted instead as `{"error":{"code":"…","message":"…"}}` so agents can parse
it (codes: `not_found`, `conflict`, `invalid`, `internal`).

Exit codes follow one contract in every output mode, mapped from the typed
error codes: **0** success, **1** internal, **2** invalid/usage (including
flag/argument mistakes), **3** not_found, **4** conflict. A partially failed
batch exits with the failure code shared by every failed id (1 when the codes
differ).

Mutating commands take a brief advisory file lock (`north/.lock`) so
concurrent `north` processes serialise; a stale lock (crashed holder) is
stolen, and a still-held one is a `conflict` after a short retry budget.

| Command | Description |
|---|---|
| `north init` | Scaffold `north/` (config with `version: 1`, `task-template.md`, `.gitattributes`, the `drafts/ tasks/ archive/` folders). Refuses (`conflict`) when a board already exists at or above the cwd. Human output ends with an "Optional next steps" epilogue (skill install, auto_commit) — suppressed under `--plain`/`--json` |
| `north task create <title> [--assignee A] [--labels ...] [--depends-on ...] [--body \| --body-file] [--plain \| --json]` | Create a task (lands in `drafts/`, status `ready`). Bodyless creates fill from `north/task-template.md` (missing/empty template ⇒ blank body) |
| `north task list [--state draft\|active\|archive\|all] [--status S] [--assignee A] [--search TEXT] [--label L] [--plain \| --json]` | List tasks (default: active). `--search` matches id/title/assignee/labels/body (case-insensitive); `--label` and `--assignee` are exact (`--label` repeatable, `--assignee ""` matches unassigned); `--sort id\|updated\|title\|assignee` + `--reverse` (default: id, newest first) |
| `north task view <id> [--plain \| --json]` | Show one task (state, status, fields + body) |
| `north task edit <id> [--title --assignee --labels --depends-on --body \| --body-file \| --append-body] [--plain \| --json]` | Edit fields/body (bumps `updated_at`). `--append-body` appends with a blank-line separator and is exclusive with `--body`/`--body-file` |
| `north task move <id[,id…]> <status> [--plain \| --json]` | Set status, in place (any → any, in any state) |
| `north task state <id[,id…]> <draft\|active\|archive> [--plain \| --json]` | Move tasks between lifecycle folders, preserving status (any → any) |
| `north task delete <id[,id…]> [-y/--yes] [--plain \| --json]` | Delete tasks. With `--plain`/`--json`, non-TTY stdin, or a batch, `-y` is required (no prompt) |
| `north board [--plain \| --json]` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS] [--plain \| --json]` | Archive active `done` tasks |
| `north doctor [--fix] [--plain \| --json]` | Board integrity check; exits non-zero (`conflict`) when unfixed issues remain |
| `north config list \| get <key> \| set <key> <value>` | Read/write `north/config.yml` settings (`auto_commit`; `version` is read-only — `set` refuses it) |
| `north skill install [--global]` | Install the agent skill (Claude Code + opencode) |
| `north skill show` | Print the embedded skill |
| `north skill check [--global]` | Compare installed skill version stamps against the binary |
| `north tui` | Interactive terminal UI (human use only) |
| `north completion <shell>` | Generate shell completions (bash/zsh/fish/powershell) |
| `north version` | Print the version (also `north --version`) |

Task ids are bare numbers: `north task view 12`. `move`, `state`, and
`delete` also take a comma-delimited batch (`north task move 2,3,4 done`):
ids are deduplicated, every id is attempted (continue-on-error) with a per-id
report (successes to stdout, `error [<code>]: …` lines to stderr; under
`--json` one `{"tasks":[…],"errors":[…]}` payload), and any failure yields a
non-zero exit. `--body-file -` reads the body from stdin (on `create` and
`edit`).

## State vs. status
`move` changes **status** (the workflow column) — in any state, with a
stderr note when the task is not active (status only shows on the board once
active). `state` changes **state** (the lifecycle folder) and preserves
status. Both are freeform — any value to any other value in one call. See
[02_board-data-model.md](02_board-data-model.md).

## Output modes
`--plain` (stable, line/tab-oriented text for scripts) and `--json` (the
`Task` dict, or board/list summaries where applicable) are supported uniformly
across the CLI. The default is human-readable. `task list --plain` columns
are `id  state  status  assignee  labels  title` (tab-separated;
assignee/labels empty when unset, labels comma-joined, title last). List/board `--json` payloads
carry a `"warnings"` array naming unparseable task files; in human/plain modes
those warnings go to stderr. For `--labels`/`--depends-on` on `edit`, passing
the flag with no values clears the field; omitting it leaves it unchanged.

## Prompts
The only interactive prompt is `task delete` without `-y`, and it goes to
stderr. In machine modes (`--plain`/`--json`) or when stdin is not a terminal,
the prompt is replaced by an `invalid` error demanding `-y`.

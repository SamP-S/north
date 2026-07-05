# 3. CLI

`north <command>`. Every command (except `init`) finds the board by walking up
from the current directory. Failures print `error: <message>` to stderr and
exit non-zero; when the failing command was invoked with `--json`, the error is
emitted instead as `{"error":{"code":"…","message":"…"}}` so agents can parse
it (codes: `not_found`, `conflict`, `invalid`, `internal`).

| Command | Description |
|---|---|
| `north init` | Scaffold `north/config.yml` + the `drafts/ tasks/ archive/` folders. Refuses (`conflict`) when a board already exists at or above the cwd |
| `north task create <title> [--agent A] [--labels ...] [--depends-on ...] [--body \| --body-file] [--plain \| --json]` | Create a task (lands in `drafts/`, status `ready`) |
| `north task list [--state draft\|active\|archive\|all] [--status S] [--search TEXT] [--label L] [--plain \| --json]` | List tasks (default: active). `--search` matches title/body/labels; `--label` is exact and repeatable |
| `north task view <id> [--plain \| --json]` | Show one task (state, status, fields + body) |
| `north task edit <id> [--title --agent --labels --depends-on --body \| --body-file \| --append-body] [--plain \| --json]` | Edit fields/body (bumps `updated_at`). `--append-body` appends with a blank-line separator and is exclusive with `--body`/`--body-file` |
| `north task move <id> <status> [--plain \| --json]` | Set status of an **active** task, in place (freeform: any → any) |
| `north task state <id> <draft\|active\|archive> [--plain \| --json]` | Move a task between lifecycle folders, preserving status (freeform: any → any) |
| `north task delete <id> [-y/--yes] [--plain \| --json]` | Delete a task. With `--plain`/`--json` or non-TTY stdin, `-y` is required (no prompt) |
| `north board [--plain \| --json]` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS] [--plain \| --json]` | Archive active `done` tasks |
| `north doctor [--fix] [--plain \| --json]` | Board integrity check; exits non-zero when unfixed issues remain |
| `north config list \| get <key> \| set <key> <value>` | Read/write `north/config.yml` settings (`auto_commit`) |
| `north skill install [--global]` | Install the agent skill (Claude Code + opencode) |
| `north skill show` | Print the embedded skill |
| `north skill check [--global]` | Compare installed skill version stamps against the binary |
| `north tui` | Interactive terminal UI (human use only) |
| `north completion <shell>` | Generate shell completions (bash/zsh/fish/powershell) |
| `north version` | Print the version (also `north --version`) |

Task ids are bare numbers: `north task view 12`. `--body-file -` reads the
body from stdin (on `create` and `edit`).

## State vs. status
`move` changes **status** (the workflow column) and only works on active
tasks. `state` changes **state** (the lifecycle folder) and preserves status.
Both are freeform — any value to any other value in one call. See
[02_board-data-model.md](02_board-data-model.md).

## Output modes
`--plain` (stable, line/tab-oriented text for scripts) and `--json` (the
`Task` dict, or board/list summaries where applicable) are supported uniformly
across the CLI. The default is human-readable. List/board `--json` payloads
carry a `"warnings"` array naming unparseable task files; in human/plain modes
those warnings go to stderr. For `--labels`/`--depends-on` on `edit`, passing
the flag with no values clears the field; omitting it leaves it unchanged.

## Prompts
The only interactive prompt is `task delete` without `-y`, and it goes to
stderr. In machine modes (`--plain`/`--json`) or when stdin is not a terminal,
the prompt is replaced by an `invalid` error demanding `-y`.

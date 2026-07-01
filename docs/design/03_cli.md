# 3. CLI

`north <command>`. Every command (except `init`) finds the board by walking up
from the current directory. Failures print `error: <message>` to stderr and exit
non-zero.

| Command | Description |
|---|---|
| `north init` | Scaffold `north/config.yml` + the `drafts/ tasks/ archive/` folders |
| `north task create <title> [--agent A] [--labels ...] [--depends-on ...] [--body \| --body-file] [--plain \| --json]` | Create a task (lands in `drafts/`, status `ready`) |
| `north task list [--state draft\|active\|archive\|all] [--status S] [--plain \| --json]` | List tasks (default: active) |
| `north task view <id> [--plain \| --json]` | Show one task (state, status, fields + body) |
| `north task edit <id> [--title --agent --labels --depends-on --body] [--plain \| --json]` | Edit fields/body (bumps `updated_at`); no `--body-file` |
| `north task move <id> <status> [--plain \| --json]` | Set status of an **active** task (in place) |
| `north task promote <id> [--plain \| --json]` | draft → active |
| `north task demote <id> [--plain \| --json]` | active → draft |
| `north task archive <id> [--plain \| --json]` | draft/active → archive |
| `north task restore <id> [--plain \| --json]` | archive → draft |
| `north task delete <id> [-y/--yes] [--plain \| --json]` | Delete a task |
| `north board [--plain \| --json]` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS] [--plain \| --json]` | Archive active `done` tasks |
| `north skill install [--global]` | Install the agent skill (Claude Code + opencode) |
| `north skill show` | Print the embedded skill |
| `north tui` | Interactive terminal UI (human use only) |
| `north version` | Print the version (also `north --version`) |

## State vs. status
`move` changes **status** (the workflow column) and only works on active tasks.
promote / demote / archive / restore change **state** (the lifecycle folder) and
preserve status. See [02_board-data-model.md](02_board-data-model.md).

## Output modes
`--plain` (stable, line/tab-oriented text for scripts) and `--json` (the `Task`
dict, or board/list summaries where applicable) are supported uniformly across
the CLI: `board`, `cleanup`, and every `task` subcommand (`create`, `view`,
`list`, `edit`, `move`, `promote`, `demote`, `archive`, `restore`, `delete`).
The default is human-readable. For `--labels`/`--depends-on` on `edit`, passing
the flag with no values clears the field; omitting it leaves it unchanged.

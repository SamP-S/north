# 3. CLI

`north <command>`. Every command (except `init`) finds the board by walking up
from the current directory. Failures print `error: <message>` to stderr and exit
non-zero.

| Command | Description |
|---|---|
| `north init` | Scaffold `north/config.yml` + the `drafts/ tasks/ archive/` folders |
| `north task create <title> [--agent A] [--labels ...] [--depends-on ...] [--body \| --body-file]` | Create a task (lands in `drafts/`, status `ready`) |
| `north task list [--state draft\|active\|archive\|all] [--status S] [--plain \| --json]` | List tasks (default: active) |
| `north task view <id> [--plain \| --json]` | Show one task (state, status, fields + body) |
| `north task edit <id> [--title --agent --labels --depends-on --body \| --body-file]` | Edit fields/body (bumps `updated_at`) |
| `north task move <id> <status>` | Set status of an **active** task (in place) |
| `north task promote <id>` | draft → active |
| `north task demote <id>` | active → draft |
| `north task archive <id>` | draft/active → archive |
| `north task restore <id>` | archive → active |
| `north task delete <id> [-y]` | Delete a task |
| `north board` | Active counts per status + draft/archive tally |
| `north cleanup [--older-than DAYS]` | Archive active `done` tasks |
| `north skill install [--global]` | Install the agent skill (Claude Code + opencode) |
| `north skill show` | Print the embedded skill |

## State vs. status
`move` changes **status** (the workflow column) and only works on active tasks.
promote / demote / archive / restore change **state** (the lifecycle folder) and
preserve status. See [02_board-data-model.md](02_board-data-model.md).

## Output modes
`task list` and `task view` support `--plain` (stable, line/tab-oriented text for
scripts) and `--json` (the `Task` dict). The default is human-readable. For
`--labels`/`--depends-on` on `edit`, passing the flag with no values clears the
field; omitting it leaves it unchanged.

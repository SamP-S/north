# 3. CLI

`north <command>`. Every command (except `init`) finds the board by walking up
from the current directory. Failures print `error: <message>` to stderr and exit
non-zero.

| Command | Description |
|---|---|
| `north init` | Scaffold `north/config.yml`, the status folders + `archive/`, and `AGENTS.md` |
| `north task create <title> [--agent A] [--labels ...] [--depends-on ...] [--body \| --body-file]` | Create a task (lands in `draft/`) |
| `north task list [--status S] [--archived] [--plain \| --json]` | List/filter tasks |
| `north task view <id> [--plain \| --json]` | Show one task (frontmatter + body) |
| `north task edit <id> [--title --agent --labels --depends-on --body \| --body-file]` | Edit fields/body (bumps `updated_at`) |
| `north task move <id> <status>` | Change status (validates the transition; moves the file) |
| `north task archive <id>` | Move a task into `archive/` |
| `north task delete <id> [-y]` | Delete a task |
| `north board` | Counts per status |
| `north cleanup [--older-than DAYS]` | Bulk-archive done tasks |
| `north mcp start \| stop \| status \| run` | Manage the on-demand MCP server |

## Output modes
`task list` and `task view` support `--plain` (stable, line/tab-oriented text for
scripts) and `--json` (the `Task` dict). The default is human-readable. For
`--labels`/`--depends-on` on `edit`, passing the flag with no values clears the
field; omitting it leaves it unchanged.

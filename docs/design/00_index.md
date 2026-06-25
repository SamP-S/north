# North — Design

North is an **in-repo Markdown task board** with a CLI, modeled on
[Backlog.md](https://github.com/MrLesk/Backlog.md). The board is a `north/`
directory committed inside your project repo; each task is a plain Markdown file.
There is no daemon and no central state — `north <cmd>` operates directly on the
files.

## Sections

| File | Contents |
|---|---|
| [01_overview.md](01_overview.md) | Purpose, principles, what North is/ isn't |
| [02_board-data-model.md](02_board-data-model.md) | The task object, the state & status axes |
| [03_cli.md](03_cli.md) | The `north` CLI |
| [04_skills.md](04_skills.md) | The installable agent skill |
| [05_configuration.md](05_configuration.md) | `north/config.yml` |
| [06_testing.md](06_testing.md) | Test strategy |

History: the previous board-service design lives under
`docs/archive/design/` (v1, v2-board-service).

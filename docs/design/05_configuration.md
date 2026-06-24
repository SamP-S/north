# 5. Configuration

## `north/config.yml`
Created by `north init`. It is both the **board-discovery marker** (North walks
up looking for it) and the home for per-board settings.

```yaml
mcp_port: 8001       # port for `north mcp`
auto_commit: false   # commit each board change locally (never pushes)
```

- `mcp_port` — the port the optional MCP server binds.
- `auto_commit` — when `true`, North runs `git add` + `git commit` of the changed
  `north/…` files after each mutation; when `false` (default) it only writes/moves
  files and leaves git to you. North never pushes or pulls.

Statuses and the `task-` id prefix are hardcoded for now (making `statuses` /
`default_status` configurable here is future work).

## Environment
| Var | Purpose |
|---|---|
| `MCP_TOKEN` | Optional bearer token required on MCP requests (secret; not in `config.yml`) |
| `NORTH_BOARD` | Overrides board discovery for the MCP server (set automatically by `north mcp`) |

`~/.north/` is used only for the MCP server's pid/log files — there is no other
global state.

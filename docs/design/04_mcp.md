# 4. MCP server (optional)

For agents that speak MCP, North can expose the board over a **single** MCP
endpoint. It is off by default and started on demand:

```
north mcp start     # detached `north mcp run`; pid/log under ~/.north/
north mcp status
north mcp stop
north mcp run        # foreground
```

- One MCP server (mcp-go) served over `net/http` at
  `http://127.0.0.1:<mcp_port>/mcp` (`mcp_port` from `north/config.yml`, default
  8001), streamable-HTTP transport.
- The board is passed to the server via the `NORTH_BOARD` env var (set by
  `north mcp`); otherwise the server walks up from its working directory.
- Optional bearer token via the `MCP_TOKEN` environment variable (a secret —
  never committed in `config.yml`). The server binds loopback only.

## Tools
All call straight into the core; a `BoardError` becomes a plain tool error.

| Tool | Purpose |
|---|---|
| `list_tasks(status?, archived?)` | List tasks (without bodies) |
| `get_task(task_id)` | One task, including its body |
| `create_task(title, agent?, labels?, depends_on?, body?)` | Create a task |
| `set_task_status(task_id, status)` | Change status (validates the transition) |
| `edit_task(task_id, ...)` | Edit fields/body |

There is no REST API — MCP is the only network surface.

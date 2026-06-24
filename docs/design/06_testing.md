# 6. Testing

Tests run against a real board scaffolded in a `tmp_path` (see
`tests/conftest.py`: the `repo` and `board` fixtures). No mocking of the
filesystem — the core is plain file I/O, so tests exercise the real thing.

| File | Covers |
|---|---|
| `test_core_board.py` | discovery (walk-up), `init` scaffolding, id allocation |
| `test_core_tasks.py` | create/read/list/edit/move/delete, the transition table, filename slugs, `updated_at` |
| `test_core_archive.py` | archive + cleanup, archive↔status orthogonality |
| `test_auto_commit.py` | `auto_commit` commits locally / is off by default |
| `test_cli.py` | CLI dispatch, `--json` output, error exit codes |
| `test_mcp.py` | the single MCP server's tools wired to the core |

Gate before merging:

```bash
uv run ruff check .
uv run mypy north
uv run pytest
```

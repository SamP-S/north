"""North core: the in-repo Markdown task board.

Plain file I/O over a `north/` directory discovered inside the user's project
repo. No daemon, no HTTP — the CLI and the optional MCP server both call into
this package directly.
"""

#!/usr/bin/env bash
# Open (or attach to) the north-design cockpit: a persistent tmux session
# running Claude Code in the cockpit workspace. Desk and phone attach to
# the same session over SSH; the workspace pins the cockpit permission
# profile, board MCP (cockpit grant), and role instructions.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COCKPIT_DIR="$REPO_ROOT/cockpit"
SESSION="${COCKPIT_SESSION:-north-design}"

if ! command -v tmux &>/dev/null; then
    echo "ERROR: tmux not found." >&2
    exit 1
fi
if ! command -v claude &>/dev/null; then
    echo "ERROR: claude (Claude Code CLI) not found." >&2
    exit 1
fi

if ! tmux has-session -t "$SESSION" 2>/dev/null; then
    tmux new-session -d -s "$SESSION" -c "$COCKPIT_DIR" claude
fi

if [[ -n "${TMUX:-}" ]]; then
    exec tmux switch-client -t "$SESSION"
fi
exec tmux attach-session -t "$SESSION"

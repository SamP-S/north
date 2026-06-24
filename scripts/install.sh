#!/usr/bin/env bash
#
# Install the `north` CLI as a local tool. North is an in-repo Markdown task
# board: there is no service to provision and nothing in your home directory —
# run `north init` inside a project repo to create its `north/` board.
#
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== North install ==="

if ! command -v uv &>/dev/null; then
    echo "ERROR: uv not found. Install uv (https://docs.astral.sh/uv/) first."
    exit 1
fi
echo "[1/2] uv: $(uv --version)"

echo "[2/2] Installing the north CLI as a tool..."
uv tool install --force "$REPO_ROOT"

echo ""
echo "=== North installed ==="
echo "Get started:  cd <your-repo> && north init && north task create \"My first task\""
echo "CLI help:     north --help"
echo "Agents (MCP): north mcp start   # optional, on demand"

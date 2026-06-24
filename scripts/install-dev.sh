#!/usr/bin/env bash
#
# Development convenience: expose the `north` CLI on your PATH by symlinking the
# editable console script from the project virtualenv. Source edits to north are
# picked up immediately (no reinstall) and there's no `uv run` overhead at call
# time.
#
# For a normal install use scripts/install.sh (or `uv tool install .`). This is
# only a dev convenience for working on north itself.
#
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

NORTH_BIN="$REPO_ROOT/.venv/bin/north"
LOCAL_BIN="$HOME/.local/bin"

echo "=== North dev CLI symlink ==="

# The console script is created by `uv sync` (run scripts/install.sh first).
if [[ ! -x "$NORTH_BIN" ]]; then
    echo "ERROR: $NORTH_BIN not found. Run 'uv sync --all-extras' (or scripts/install.sh) first."
    exit 1
fi

mkdir -p "$LOCAL_BIN"
ln -sf "$NORTH_BIN" "$LOCAL_BIN/north"
echo "Linked $LOCAL_BIN/north -> $NORTH_BIN"

case ":$PATH:" in
    *":$LOCAL_BIN:"*) ;;
    *) echo "WARNING: $LOCAL_BIN is not on your PATH — add it to use 'north' directly." ;;
esac

echo "Done. Try: north --help"

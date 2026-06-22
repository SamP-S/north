#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load shared env file if present
if [[ -f "$HOME/.north/.env" ]]; then
    set -a
    # shellcheck source=/dev/null
    source "$HOME/.north/.env"
    set +a
fi

NORTH_HOME="${NORTH_HOME:-$HOME/.north}"
AURORA_HOME="${AURORA_HOME:-$NORTH_HOME/aurora}"
AURORA_PORT="${AURORA_PORT:-8000}"
BOREALIS_PORT="${BOREALIS_PORT:-8001}"
HEADLESS="${HEADLESS:-0}"

AURORA_DIR="$REPO_ROOT/aurora"
BOREALIS_DIR="$REPO_ROOT/borealis"

echo "=== North Install ==="

# Step 1: uv (provides a compatible Python >=3.12; `uv sync` downloads one if needed)
if ! command -v uv &>/dev/null; then
    echo "ERROR: uv not found. Install uv (https://docs.astral.sh/uv/) before running this script."
    exit 1
fi
echo "[1/10] uv: $(uv --version)"

# Step 2: uv sync (--all-extras pulls in the dev tools: ruff, mypy, pytest)
echo "[2/10] Installing dependencies with uv..."
cd "$REPO_ROOT"
uv sync --all-extras

# Step 3: Claude Code CLI
if command -v claude &>/dev/null; then
    echo "[3/10] Claude Code CLI already installed: $(claude --version 2>/dev/null || echo 'unknown version')"
else
    echo "[3/10] Installing Claude Code CLI..."
    curl -fsSL https://claude.ai/install.sh | bash
fi

# Step 4: Opencode CLI (pinned agent runtime)
OPENCODE_VERSION="${OPENCODE_VERSION:-1.15.13}"
if command -v opencode &>/dev/null && [[ "$(opencode --version 2>/dev/null)" == "$OPENCODE_VERSION" ]]; then
    echo "[4/10] Opencode $OPENCODE_VERSION already installed"
else
    echo "[4/10] Installing opencode $OPENCODE_VERSION..."
    curl -fsSL https://opencode.ai/install | VERSION="$OPENCODE_VERSION" bash
fi

# Step 5: Auth
if claude auth status &>/dev/null 2>&1; then
    echo "[5/10] Already authenticated with Claude."
elif [[ "$HEADLESS" == "1" ]]; then
    echo "[5/10] Headless mode: set CLAUDE_CODE_OAUTH_TOKEN in $NORTH_HOME/.env before starting the service."
else
    echo "[5/10] Authentication..."
    claude auth login
fi

# Step 6: Create aurora home dirs
if [[ -d "$AURORA_HOME/repos" && -d "$AURORA_HOME/worktrees" ]]; then
    echo "[6/10] Aurora home directories already exist at $AURORA_HOME"
else
    echo "[6/10] Creating aurora home directories at $AURORA_HOME..."
    mkdir -p "$AURORA_HOME/repos" "$AURORA_HOME/worktrees"
fi

# Step 7: Clone board repo
BOARD_PATH="${BOARD_PATH:-$NORTH_HOME/borealis/board}"
mkdir -p "$(dirname "$BOARD_PATH")"
if [[ -d "$BOARD_PATH/.git" ]]; then
    echo "[7/10] Board repo already present at $BOARD_PATH"
else
    echo "[7/10] Setting up board repo..."
    if [[ -z "${BOARD_REPO_SSH_URL:-}" ]]; then
        echo "ERROR: BOARD_REPO_SSH_URL is not set. Add it to $NORTH_HOME/.env and re-run install."
        exit 1
    fi
    if ! git clone "$BOARD_REPO_SSH_URL" "$BOARD_PATH"; then
        echo "ERROR: Failed to clone board repo from $BOARD_REPO_SSH_URL"
        echo "Ensure the remote exists and SSH access is configured, then re-run install."
        exit 1
    fi
fi

# Step 8: Enable linger
if loginctl show-user "$(whoami)" 2>/dev/null | grep -q "Linger=yes"; then
    echo "[8/10] User linger already enabled"
else
    echo "[8/10] Enabling user linger..."
    loginctl enable-linger "$(whoami)"
fi

# Step 9: Install systemd units
SYSTEMD_USER_DIR="$HOME/.config/systemd/user"
mkdir -p "$SYSTEMD_USER_DIR"
UNITS_CHANGED=0

_install_unit() {
    local src="$1" dst="$2" working_dir="$3"
    local tmp
    tmp=$(mktemp)
    sed "s|@@WORKING_DIR@@|${working_dir}|g" "$src" > "$tmp"
    if [[ ! -f "$dst" ]] || ! diff -q "$tmp" "$dst" &>/dev/null; then
        mv "$tmp" "$dst"
        UNITS_CHANGED=1
    else
        rm "$tmp"
    fi
}

_install_unit "$REPO_ROOT/systemd/aurora.service"   "$SYSTEMD_USER_DIR/aurora.service"   "$AURORA_DIR"
_install_unit "$REPO_ROOT/systemd/borealis.service" "$SYSTEMD_USER_DIR/borealis.service" "$BOREALIS_DIR"
# failure-notification template (OnFailure= on all North units); the curl
# script lives in the repo, so the repo root is its working-dir substitution
_install_unit "$REPO_ROOT/systemd/north-notify-failure@.service" \
    "$SYSTEMD_USER_DIR/north-notify-failure@.service" "$REPO_ROOT"
# ollama is NOT managed by North — it's an optional external provider (see plan 029)
cp "$REPO_ROOT/systemd/opencode.service" "$SYSTEMD_USER_DIR/opencode.service"

if [[ "$UNITS_CHANGED" == "1" ]]; then
    echo "[9/10] Systemd units updated, reloading..."
    systemctl --user daemon-reload
else
    echo "[9/10] Systemd units already up to date"
fi
systemctl --user enable --now aurora.service borealis.service opencode.service

# Step 10: Check optional ollama provider (warn only — ollama is external + optional)
echo "[10/10] Checking ollama (optional, for local models)..."
OLLAMA_URL="${OLLAMA_URL:-http://127.0.0.1:11434}"
if curl -fsS -m 5 "${OLLAMA_URL}/api/tags" -o /tmp/north-ollama-tags 2>/dev/null; then
    for model in "mistral:7b" "codellama:7b"; do
        if ! grep -q "$model" /tmp/north-ollama-tags; then
            echo "WARNING: ollama model '$model' not pulled. Run: ollama pull $model"
        fi
    done
    rm -f /tmp/north-ollama-tags
else
    echo "NOTE: no ollama reachable at ${OLLAMA_URL}. Local-model pipelines will defer"
    echo "      until you start ollama (see README); cloud pipelines are unaffected."
fi

# OAuth smoke test
echo "Running OAuth smoke test..."
if ! uv run --package aurora python -c "
import asyncio
from claude_agent_sdk import query
async def _test():
    async for _ in query(prompt='Say hello'):
        pass
asyncio.run(_test())
"; then
    echo "ERROR: OAuth smoke test FAILED. Check your Claude credentials and re-run install."
    exit 1
fi
echo "OAuth smoke test passed."

echo ""
echo "=== North installed successfully ==="
echo "Aurora:   http://127.0.0.1:${AURORA_PORT}"
echo "Borealis: http://127.0.0.1:${BOREALIS_PORT}"
echo "Logs:     journalctl --user -u aurora.service -f"
echo "          journalctl --user -u borealis.service -f"

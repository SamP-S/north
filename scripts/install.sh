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
NORTH_PORT="${NORTH_PORT:-8001}"

echo "=== North Install ==="

# Step 1: uv (provides a compatible Python >=3.12; `uv sync` downloads one if needed)
if ! command -v uv &>/dev/null; then
    echo "ERROR: uv not found. Install uv (https://docs.astral.sh/uv/) before running this script."
    exit 1
fi
echo "[1/6] uv: $(uv --version)"

# Step 2: uv sync (--all-extras pulls in the dev tools: ruff, mypy, pytest)
echo "[2/6] Installing dependencies with uv..."
cd "$REPO_ROOT"
uv sync --all-extras

# Step 3: Install the `north` CLI as a local tool (exposes `north` on PATH)
echo "[3/6] Installing the north CLI..."
uv tool install --force "$REPO_ROOT"

# Step 4: Clone board repo
BOARD_PATH="${BOARD_PATH:-$NORTH_HOME/board}"
mkdir -p "$(dirname "$BOARD_PATH")"
if [[ -d "$BOARD_PATH/.git" ]]; then
    echo "[4/6] Board repo already present at $BOARD_PATH"
else
    echo "[4/6] Setting up board repo..."
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

# Step 5: Enable linger (so the user service keeps running across logout/boot)
if loginctl show-user "$(whoami)" 2>/dev/null | grep -q "Linger=yes"; then
    echo "[5/6] User linger already enabled"
else
    echo "[5/6] Enabling user linger..."
    loginctl enable-linger "$(whoami)"
fi

# Step 6: Install + enable systemd units
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

_install_unit "$REPO_ROOT/systemd/north.service" "$SYSTEMD_USER_DIR/north.service" "$REPO_ROOT"

if [[ "$UNITS_CHANGED" == "1" ]]; then
    echo "[6/6] Systemd units updated, reloading..."
    systemctl --user daemon-reload
else
    echo "[6/6] Systemd units already up to date"
fi
systemctl --user enable --now north.service

echo ""
echo "=== North installed successfully ==="
echo "North: http://127.0.0.1:${NORTH_PORT}"
echo "Logs:  journalctl --user -u north.service -f"
echo "CLI:   north --help"

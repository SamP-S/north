#!/usr/bin/env bash
# Telegram ping when a North unit hard-fails (systemd OnFailure=, health layer b).
# Runs outside the failed process, so it works even when Python can't self-report.
set -euo pipefail

UNIT="${1:-unknown}"

if [[ -f "$HOME/.north/.env" ]]; then
    set -a
    # shellcheck source=/dev/null
    source "$HOME/.north/.env"
    set +a
fi

TEXT="[unit_failed] ${UNIT} failed on $(hostname) at $(date -Is)"

if [[ -n "${TELEGRAM_BOT_TOKEN:-}" && -n "${TELEGRAM_CHAT_ID:-}" ]]; then
    curl -fsS -m 10 "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
        -d "chat_id=${TELEGRAM_CHAT_ID}" --data-urlencode "text=${TEXT}" >/dev/null \
        || logger -t north-notify "telegram send failed: ${TEXT}"
else
    logger -t north-notify "telegram unconfigured: ${TEXT}"
fi

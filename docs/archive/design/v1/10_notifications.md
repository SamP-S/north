## 10. Notifications (Telegram)

Outbound only. Configured via `TELEGRAM_BOT_TOKEN` + `TELEGRAM_CHAT_ID` in `.env`. A single `notifications.py` sender is enqueued by the runner.

**Dedup:** keyed by `(event_type, task_id)` within a single task run — duplicate events for the same task run are dropped before sending.

**Retry:** 3 attempts with a 5s fixed delay between each. If all attempts fail, the failure is logged and the notification is dropped — the operator can check `aurora status` for current state.

### 10.1 Events

- Invalid `_feature.md` frontmatter detected — includes filename and validation error
- Task completed — status: `done`
- Task terminal failure — status: `failed` or `blocked`; includes reason (e.g. missing `pipeline` field, invalid frontmatter, agent returned `BLOCKED`, infrastructure failure) and count of dependent tasks that will remain queued until resolved
- Feature ready for review — all tasks done; operator should run `aurora approve`, `aurora rollback`, or `aurora reject`
- Feature merged — `aurora approve` succeeded; board archived
- Feature rolled back — `aurora rollback` run; tasks re-queued
- Feature rejected — `aurora reject` run; board archived
- Feature approve conflict — `aurora approve` encountered merge conflicts; operator must resolve manually
- Approaching monthly credit soft cap
- Rate-limit rejected / credit exhausted
- **OAuth authentication failed** — re-run `claude auth login`
- **Board push conflict** — rebase conflict after concurrent operator push; operator must resolve manually and push

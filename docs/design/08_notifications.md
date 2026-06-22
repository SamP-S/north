## 8. Notifications

Outbound only. The transport is config: `log` (default) or `telegram` (degrades to
log when `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` are unconfigured). A single
notifier in North is enqueued on a background thread and never blocks or fails a
board mutation.

**Dedup:** identical `(kind, fields)` events collapse to one send within
`NOTIFY_DEDUPE_WINDOW_S` (default `300`). A global `NOTIFY_RATE_LIMIT_PER_MIN`
(default `20`) caps sends so a retry loop can never storm the channel.

**Service health:** WARNING+ log records forward through the notifier (deduped per
logger + message template over `LOG_NOTIFY_DEDUPE_WINDOW_S`, default `3600`).

ntfy (self-hosted) is the recorded swap candidate — the transport interface makes
that a config change.

### 8.1 Events

- Conversation shipped / decomposed
- Task blocked on a question
- Task terminal failure (`failed` / `blocked`) — includes the reason and the
  count of dependent tasks that remain queued until resolved
- Feature ready for review — all tasks `done` (the `north/brief` note landing on
  the feature thread is what emits this `feature_review` event)
- Invalid `_feature.md` frontmatter detected — includes filename and validation
  error
- Board push conflict — rebase conflict after a concurrent operator push; the
  operator must resolve manually and push

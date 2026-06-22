# 027 — Borealis client keep-alive race fix

## Context

Aurora's supervisor was emitting an ERROR every few minutes:

```
aurora.service.orchestrator.supervisor: Failed to fetch pending conversations
httpcore.RemoteProtocolError: Server disconnected without sending a response.
```

`NotifyLogHandler` forwards ERROR+ to Telegram (deduped hourly via
`log_notify_dedupe_window_s`), so it surfaced as a notification roughly every
couple of hours despite firing ~89×/2 days.

**Root cause — an idle keep-alive timeout tie.** Three values were all `5s`:

- client `httpx` `keepalive_expiry` (the default, `BorealisClient` set no limits)
- Borealis uvicorn `--timeout-keep-alive` (the default; launched with no flag)
- Aurora's `poll_interval_s`

A pooled connection idles ~5s between polls — exactly the server's keep-alive
deadline. With client expiry == server timeout there is no ordering guarantee,
so when the server wins the tie it closes the socket; Aurora's next request
goes down the dead socket → `RemoteProtocolError`. The blip is benign (the next
poll succeeds) but spams ERROR/Telegram. `get_pending_conversations` (and the
other reads) had no retry, unlike `update_task_status`.

## Scope

- **Root cause:** make the client recycle idle connections *before* the server
  closes them, eliminating the race structurally.
- **Safety net:** retry GETs on transient transport errors (covers genuine
  blips: server restart, real network hiccup).
- Document the controlling invariant so future edits can't silently reintroduce
  the bug.

## The invariant (constraint note)

> **`client keepalive_expiry < server timeout-keep-alive`** must always hold.

This — not the poll rate — is what prevents the race. The race window
(`server_keepalive ≤ idle < client_keepalive_expiry`) is non-empty *only* when
this invariant is violated, so it is independent of `poll_interval_s`:

- Faster polling: idle < client expiry → client reuses a still-live connection.
- Slower polling (even beyond the server timeout): idle exceeds client expiry →
  client discards and opens a fresh connection before ever reusing a closed one.

The poll rate may therefore change freely. The only thing that would
reintroduce the bug is editing the two keep-alive values such that
`client_keepalive_expiry ≥ server_timeout`. Current values: client `5.0s`,
server `30s`.

## Files modified

- `aurora/aurora/service/borealis_client.py`
  - `httpx.Limits(keepalive_expiry=5.0)` on the `AsyncClient` (`_KEEPALIVE_EXPIRY_S`).
  - New `_get()` helper: retries `httpx.TransportError` on a fresh connection
    (`_GET_RETRIES = 3`, reuses `_RETRY_BACKOFF_S`); HTTP status errors are not
    retried. All 12 GET methods routed through it.
- `systemd/borealis.service` — `--timeout-keep-alive 30` added to `ExecStart`.
- `aurora/tests/test_borealis_client.py` — `test_get_retries_transient_transport_error`.

## Todo

- [x] 1. Diagnose root cause from `/tmp/aurora.log` traceback + keep-alive timings
- [x] 2. Client: `keepalive_expiry` limit + `_get()` retry wrapper for all GETs
- [x] 3. Server: `--timeout-keep-alive 30` in `systemd/borealis.service`
- [x] 4. Test for transient-transport-error retry; full suite green
- [x] 5. Lint/format/mypy (no net-new errors); document the invariant
- [x] 6. Apply live: restart Borealis (with the flag) and Aurora to pick up changes

## Verification

- `uv run pytest aurora/tests` → 128 passed (incl. the new retry test).
- `ruff check` clean; `mypy` holds at the pre-existing 30-error baseline.
- Live: after restart, `/tmp/aurora.log` should stop logging
  "Failed to fetch pending conversations" and Telegram should go quiet.

## Change history

- [2026-06-15] Diagnosed the keep-alive tie; implemented client `keepalive_expiry`
  + GET retry wrapper, server `--timeout-keep-alive 30`, and the retry test.
  Plan created. Live restart (todo 6) still pending.
- [2026-06-15] Restarted both services live via `nohup` (Borealis now runs with
  `--timeout-keep-alive 30`). Aurora polling Borealis cleanly; zero "Failed to
  fetch pending conversations" since restart.

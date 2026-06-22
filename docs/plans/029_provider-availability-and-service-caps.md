# 029 — Provider availability gating + service-restart caps

## Context

Two related hardening needs surfaced while bringing North up under systemd:

- **ollama crash-loop / over-coupling.** North shipped a user-level
  `ollama.service`, but the standard ollama install is a *system* service already
  bound to `:11434` → the user unit crash-looped on the port (`Restart=on-failure`,
  no start limit) and spammed `OnFailure=` notifications. More fundamentally, per
  [[021_session-pipeline-architecture]] ollama is just **one provider among many** —
  pipelines blend local (ollama) and cloud models per stage/profile
  (`profile.provider or settings.opencode_local_provider`, `session.py:137`). North
  should not own or require ollama.
- **No bounded failure.** All units restart forever with no cap.

This plan makes ollama an **optional, externally-provided provider**, gates work on
**per-provider availability** (token-free), and caps unit restarts.

## Scope

- North stops managing ollama (unit, deps, install enable). install warns, never
  errors, never auto-installs.
- New `ollama_url` setting (default `http://127.0.0.1:11434`).
- A token-free per-provider availability check; if a task needs an unavailable
  provider, defer that task **internally** (no board status change) while letting
  independent tasks run; auto-resume when the provider returns.
- `StartLimitBurst=3` on the remaining units (already applied; see Change History).

## Design (numbered for reference)

1. **No North-managed ollama.** Delete `systemd/ollama.service`; remove
   `Wants=/After=ollama.service` from `aurora.service` + `opencode.service`; drop
   ollama from install.sh's `enable --now` list. (Installing optional services like
   ollama is deferred to a future north *setup* CLI — distinct from the per-service
   runtime CLIs in [[028_cli-service-control]].)
2. **`ollama_url`** in aurora `config.py`, default `http://127.0.0.1:11434`. Used by
   the availability probe; documented that opencode's ollama provider must match it.
3. **Token-free per-provider availability check.** New
   `aurora/aurora/service/runtime/availability.py`: `async provider_available(
   provider, settings) -> bool`. Registry by provider id; ollama probes
   `GET {ollama_url}/api/tags` (no inference, no tokens). Providers with no
   token-free check (cloud) are treated as **available** for now.
4. **3 attempts.** The probe retries up to 3 total with short backoff before
   declaring a provider unavailable.
5. **Pickup-time gating (Option B), internal only.** When the supervisor scans the
   queue, it resolves the providers a task's pipeline still needs (load pipeline →
   for each remaining `RunStage`, `load_profile` → resolved provider; gates need
   none) and **skips** any task whose needed provider is currently unavailable,
   recording it in an in-memory deferral set keyed by `(task, provider)` — mirroring
   the existing `_decompose_backoff` pattern. The task is **never set `in_progress`**
   and its board status stays `queued`; the block is purely aurora-internal.
   Independent tasks (e.g. cloud-only) are evaluated and run normally.
6. **Auto-recover.** The supervisor periodically re-probes providers it has deferred
   tasks on; when one returns, its deferrals clear and those tasks resume from their
   pipeline checkpoint (021 checkpointing). A task also clears if its `pipeline`
   field is changed to no longer need that provider.
7. **Notify on provider transitions, not per task.** One notification when a provider
   first goes unavailable ("deferring tasks: ollama unreachable") and one when it
   returns ("resuming: ollama back"), to avoid per-task noise. Distinct from a manual
   `aurora pause`.
8. **`Outcome.PROVIDER_UNAVAILABLE`** added to `runtime/__init__.py` for the in-flight
   safety net: if a provider drops *mid-pipeline* (passed pickup but failed at session
   creation), the stage runner returns this outcome and the task is re-queued +
   deferred rather than failed.
9. **install.sh + units:** reachability and model-presence (mistral/codellama) stay
   **warnings**; reword the "ensure ollama.service is running" hint to point at the
   external ollama / README. `StartLimitBurst=3` already on aurora/borealis/opencode.

## Files

- `systemd/ollama.service` — **delete**.
- `systemd/aurora.service`, `systemd/opencode.service` — drop `Wants=/After=ollama.service`.
- `scripts/install.sh` — remove ollama unit `cp` + drop from `enable --now`; keep
  reachability/model checks as warnings; reword hint.
- `aurora/aurora/service/config.py` — add `ollama_url`.
- `aurora/aurora/service/runtime/availability.py` — **new**: token-free probe + registry + 3-try.
- `aurora/aurora/service/runtime/__init__.py` — add `Outcome.PROVIDER_UNAVAILABLE`.
- `aurora/aurora/service/runtime/session.py` / `stages/runner.py` — gate session
  creation on availability; surface the new outcome.
- `aurora/aurora/service/orchestrator/supervisor.py` (+ `task_runner.py`) — pickup-time
  provider resolution, internal deferral set, re-probe/auto-resume, transition notify.
- `aurora/tests/` — availability probe (mock httpx), deferral/gating, outcome handling.
- `README.md` — ollama is an optional prerequisite for local models.

## Todos

- [x] 1. Remove North's ollama ownership: delete unit; strip `Wants=/After=` from aurora+opencode; drop from install.sh `enable`
- [x] 2. install.sh: keep ollama reachability + models as warnings; reword hint (now probes `${OLLAMA_URL}/api/tags`)
- [x] 3. config: add `ollama_url` (+ `provider_check_retries`)
- [x] 4. `availability.py`: token-free `provider_available` + registry (ollama probe) + 3-try; `required_providers` resolver
- [x] 5. `Outcome.PROVIDER_UNAVAILABLE`; gate session creation in `SessionRunner`; stage runner maps it to `READY`
- [x] 6. Supervisor `select_runnable`: pickup-time provider resolution; skip tasks needing a down provider; board stays `queued`
- [x] 7. Supervisor: re-probe each tick (auto-resume); one-shot down/up transition notify (`provider:down` / `provider:up`)
- [x] 8. Tests (`test_provider_availability.py`: probe retry/give-up, resolver, supervisor select + notify)
- [x] 9. README: ollama optional; lint/format/mypy clean; full suites 302 passed
- [x] 10. Live: removed stale installed `ollama.service`; aurora/opencode no longer reference ollama; probe smoke (system ollama reachable; bad url fails; cloud assumed available)

## Open detail decisions (recommendations; confirm during impl)

- **D1 — pickup-time vs lazy mid-pipeline.** Recommend **pickup-time** resolution
  (design pt 5) so the board never shows a false `in_progress`; pt 8 is only the
  in-flight safety net. (Alternative: lazy — run until the down provider, then
  re-queue; simpler but flips status briefly.)
- **D2 — notify granularity.** Recommend **per-provider transition** (pt 7), not per task.
- **D3 — cloud availability.** No token-free cloud check today → treat cloud providers
  as always-available; only ollama (and future probe-able locals) get real checks.

## Verification

- `uv run pytest aurora/tests borealis/tests` green; ruff + mypy clean.
- Live (ollama deliberately stopped): a cloud-only task runs to completion; a task
  whose pipeline needs ollama stays `queued` on the board and does not flip to
  `in_progress`; one "ollama unreachable" notification fires; starting ollama
  auto-resumes the deferred task and fires one "ollama back" notification.

## Change History

- [2026-06-15] Plan created. Direction: ollama is an optional, externally-provided
  provider; per-provider token-free availability gating with internal per-task
  deferral (Option B, no board status change, auto-recover); install warns only;
  `StartLimitBurst=3` on units.
- [2026-06-15] Pre-work already on branch (to be folded into this plan's commit set):
  `StartLimitBurst=3` added to all four unit templates + applied live;
  `ollama.service` user-unit stopped (system ollama already serves `:11434`).
- [2026-06-15] Implemented todos 1–10. North no longer manages ollama (unit deleted,
  deps stripped, install warns only). New `runtime/availability.py` (token-free probe
  + `required_providers`), `ollama_url`/`provider_check_retries` config,
  `Outcome.PROVIDER_UNAVAILABLE`, per-session gate in `SessionRunner` (→ `READY`),
  supervisor `select_runnable` pickup-gating with down/up transition notifications.
  302 tests pass, ruff + mypy clean. Live: stale ollama unit removed; probe verified
  against the system ollama. Decisions taken: D1 pickup-time, D2 per-provider notify,
  D3 cloud-assumed-available.

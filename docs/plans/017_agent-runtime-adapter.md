# 017 — Agent Runtime Adapter

## Summary

First step of the v2 architecture (`docs/design/01_v2-architecture.md`):
decouple workflow automation from agent execution inside Aurora by introducing
an `AgentRuntime` adapter interface. No behaviour change — the existing
cloud/local execution paths are wrapped as `LegacyRuntime`. This creates the
seam that `018_opencode-runtime` plugs into.

Also cleans up the entanglement this split exposes:

- `task_ingest` smuggles the pipeline via `task_state.__dict__["pipeline_def"]`
  → return the `PipelineDefinition` explicitly and pass it as a parameter.
- `run_pipeline` branches on `_infer_provider(agent_def.model)` and handles
  `rate_limited`/`auth_failed` via result-specific fields → replaced by a
  single `runtime.run_step()` call returning a `StepResult` with an `Outcome`
  enum (`ok | rate_limited | auth_failed | timeout | error`).
- `GLOBAL_BASH_DENYLIST` lives in `cloud.py` but is imported by `tools.py`
  → move to a shared `execution/policy.py`.

## New Module Layout (aurora)

```
aurora/service/
  runtime/
    __init__.py        # AgentRuntime Protocol, StepRequest, StepResult, Outcome
    legacy.py          # LegacyRuntime wrapping run_cloud_step / run_local_step
  pipeline/
    executor.py        # run_pipeline(pipeline, task_state, roster, runtime, ...)
  execution/           # unchanged internals, now only referenced by legacy.py
    policy.py          # GLOBAL_BASH_DENYLIST (moved from cloud.py)
```

## Files to Modify

- `aurora/aurora/service/runtime/__init__.py` — new: protocol + dataclasses
- `aurora/aurora/service/runtime/legacy.py` — new: `LegacyRuntime`
  (provider inference moves here from `executor.py`)
- `aurora/aurora/service/pipeline/executor.py` — `task_ingest` returns
  `(status, pipeline | None)`; `run_pipeline` takes `pipeline` and `runtime`
  params; drop provider branching and SDK/Ollama imports
- `aurora/aurora/service/execution/policy.py` — new: denylist
- `aurora/aurora/service/execution/cloud.py`, `tools.py` — import denylist
  from `policy.py`
- `aurora/aurora/service/orchestrator/task_runner.py` — hold pipeline locally
  instead of `task_state.__dict__`; construct runtime from settings and pass
  to `run_pipeline`
- `aurora/aurora/service/config.py` — add `agent_runtime: str = "legacy"`
- `aurora/tests/test_pipeline_executor.py` — drive `run_pipeline` with a fake
  `AgentRuntime` instead of patched cloud/local functions
- `aurora/tests/integration/test_rate_limit.py` — also sets
  `__dict__["pipeline_def"]` (line 60); update to the explicit pipeline param
- `aurora/tests/test_cloud.py`, `test_local_executor.py`, `test_tools.py` —
  unchanged behaviour; fix imports only

## Todo

- [x] 1. Add `runtime/__init__.py` (Protocol, `StepRequest`, `StepResult`,
      `Outcome`)
- [x] 2. Move denylist to `execution/policy.py`; update imports
- [x] 3. Implement `LegacyRuntime` (provider inference + cloud/local dispatch +
      result mapping; note: cloud/local fold timeouts into a generic failed
      result internally, so legacy maps both to `Outcome.error` — `timeout` is
      only distinguishable from 018 onward)
- [x] 4. Refactor `executor.py`: explicit pipeline return/param, runtime param,
      outcome-based control flow
- [x] 5. Update `task_runner.py` and `config.py` (runtime selection)
- [x] 6. Update tests; full suite + ruff

## Change History

- [2026-06-11] Plan created
- [2026-06-11] Review amendments: added `tests/integration/test_rate_limit.py`
  to files to modify; noted legacy runtime maps timeouts to `Outcome.error`
- [2026-06-11] All items implemented. 70 tests pass, ruff clean; mypy errors
  reduced 17 → 14 (all remaining pre-exist in `borealis_client.py`/`review.py`).
  Design deviation: `StepResult` carries the parsed `artifact` instead of raw
  `text` (legacy cloud/local parse internally and the plan limits them to
  import-only changes); parsing moves into the executor with 018's cutover.
  Pre-existing failures untouched (out of scope): `test_full_task_run.py` and
  `test_pause_resume.py` import the removed `TaskModel` (collection error);
  `test_feature_lifecycle.py` (2) and `test_review.py::test_reject_resets_and_closes`
  fail on the clean tree too. New tests added: ingest-missing-pipeline,
  rate-limited→queued, auth-failed→blocked via a fake `AgentRuntime`.

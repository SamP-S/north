# 010 — API Improvements

## Summary

Four targeted improvements to the Borealis REST API:
1. Restructure URL paths to a proper resource hierarchy under `/api/projects/{project}/...`
2. Replace per-router global state with FastAPI dependency injection
3. Add a `GET /api/projects/{project}/features/{feature}/tasks` list endpoint
4. Validate status values at the API boundary using the `FeatureStatus`/`TaskStatus` enums

Top-level convenience endpoints (`/api/queue`, `/api/review`, `/api/events`, `/api/status`, `/api/health`) remain flat.

## New URL Structure

```
GET    /api/projects
POST   /api/projects/register
GET    /api/projects/{project}
DELETE /api/projects/{project}

GET    /api/projects/{project}/features
PATCH  /api/projects/{project}/features/{feature}/status
POST   /api/projects/{project}/features/{feature}/requeue
GET    /api/projects/{project}/features/{feature}/tasks          ← NEW
GET    /api/projects/{project}/features/{feature}/tasks/{task_id}
PATCH  /api/projects/{project}/features/{feature}/tasks/{task_id}/status
```

## Files to Modify

- `borealis/service/api/tasks.py` — new paths, DI, status validation
- `borealis/service/api/features.py` — new paths, DI, status validation
- `borealis/service/main.py` — introduce `deps.py` wiring, update project routes, add tasks list endpoint
- `borealis/service/api/deps.py` — NEW: shared dependency provider
- `borealis/cli/commands/observe.py` — update URLs
- `borealis/cli/commands/projects.py` — update URLs
- `tests/test_api.py` — update fixture setup and URL paths

## Todo

- [x] 1. Create `borealis/service/api/deps.py` with `get_board_context` dependency
- [x] 2. Refactor `api/tasks.py`: new paths, use DI, validate status
- [x] 3. Refactor `api/features.py`: new paths, use DI, validate status
- [x] 4. Update `service/main.py`: wire DI, update project routes to new paths
- [x] 5. Add `GET .../tasks` list endpoint (in tasks.py)
- [x] 6. Update CLI `observe.py` and `projects.py` for new URLs
- [x] 7. Update `tests/test_api.py` for new setup and URLs

## Change History

- [2026-06-09] Plan created
- [2026-06-09] All changes implemented, 34/34 tests passing

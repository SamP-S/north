# 011 — CRUD Endpoints for Tasks and Features

## Summary

Add full CRUD operations for tasks and features. IDs are borealis-managed and cannot be set or changed by the caller:
- **Task IDs**: auto-incremented zero-padded integers (`001`, `002`, ...) within a feature
- **Feature IDs**: slugified from title (`"Add login"` → `add-login`)

## New Endpoints

```
# Features
POST   /api/projects/{project}/features                         create feature
PUT    /api/projects/{project}/features/{feature}               full update (id blocked)

# Tasks
POST   /api/projects/{project}/features/{feature}/tasks         create task
PUT    /api/projects/{project}/features/{feature}/tasks/{task_id}  full update (id blocked)
DELETE /api/projects/{project}/features/{feature}/tasks/{task_id}  delete task file + commit
```

## Request Bodies

**POST /api/projects/{project}/features**
```json
{ "title": "str", "epic": "str", "description": "str?", "depends_on": ["str"] }
```
- Generates `feature_id` by slugifying title (lowercase, spaces/special chars → `-`, deduplicate `-`)
- `branch` is derived automatically — same as `feature_id`
- Creates `board/features/active/{feature_id}/_feature.md` and `tasks/` dir
- Initial status: `open`

**PUT /api/projects/{project}/features/{feature}**
```json
{ "title": "str", "epic": "str", "description": "str?", "depends_on": ["str"], "status": "str" }
```
- Returns 404 if feature does not exist
- Rewrites all frontmatter fields except `id` and `branch`; validates `status` against `FeatureStatus`

**POST /api/projects/{project}/features/{feature}/tasks**
```json
{ "title": "str", "pipeline": "str", "epic": "str", "body": "str?", "depends_on": ["str"] }
```
- Auto-increments task ID by scanning existing `.md` files in `tasks/` dir
- Creates `tasks/{id}.md`; initial status: `draft`

**PUT /api/projects/{project}/features/{feature}/tasks/{task_id}**
```json
{ "title": "str", "pipeline": "str", "epic": "str", "body": "str?", "depends_on": ["str"], "status": "str" }
```
- Returns 404 if task does not exist
- Rewrites all frontmatter fields except `id`; validates `status` against `TaskStatus`

**DELETE /api/projects/{project}/features/{feature}/tasks/{task_id}**
- Deletes `tasks/{task_id}.md` (and `{task_id}.result.md` if present)
- Commits removal to board; allowed regardless of task status

## Writer Additions Needed

Add to `board/writer.py`:
- `write_task_file(path, frontmatter_dict, body)` — write a new task `.md` file
- `write_feature_file(path, frontmatter_dict, description)` — write a new `_feature.md` file
- `delete_task_file(board_path, task_path, result_path?)` — remove and commit

## Files to Modify

- `borealis/service/board/writer.py` — add write/delete helpers
- `borealis/service/api/tasks.py` — add POST, PUT, DELETE endpoints
- `borealis/service/api/features.py` — add POST, PUT endpoints
- `borealis/tests/test_api.py` — tests for all new endpoints

## Todo

- [x] 1. Add `write_task_file`, `write_feature_file`, `delete_task_files` to `board/writer.py`
- [x] 2. Add `POST` and `PUT` to `api/features.py`
- [x] 3. Add `POST`, `PUT`, `DELETE` to `api/tasks.py`
- [x] 4. Update `tests/test_api.py` with tests for all new endpoints

## Change History

- [2026-06-09] Plan created
- [2026-06-09] All changes implemented, 25/25 tests passing

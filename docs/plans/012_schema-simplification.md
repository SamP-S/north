# 012 — Schema Simplification

## Summary

Remove redundant/derivable fields from models, frontmatter, and API surfaces.

## Field Changes

| Model | Field | Action |
|-------|-------|--------|
| `ProjectModel` | `base_branch` | Drop entirely |
| `FeatureModel` | `epic` | Drop entirely |
| `FeatureModel` | `branch` | Drop stored field → `@property` returning `self.feature_id` |
| `TaskModel` | `epic` | Drop entirely |
| `TaskModel` | `project` | Drop stored field → `@property` returning `self.task_path.parts[-7]` |
| `TaskModel` | `branch` | Drop stored field → `@property` returning `self.feature` |
| `TaskModel` | `attempts` | Drop entirely |

## Files to Modify

- `borealis/service/models.py` — remove fields, add properties
- `borealis/service/board/parser.py` — stop reading dropped fields from frontmatter
- `borealis/service/board/writer.py` — stop writing dropped fields to frontmatter
- `borealis/service/api/features.py` — remove from request bodies and responses
- `borealis/service/api/tasks.py` — remove from request bodies and responses
- `borealis/service/main.py` — update list/queue/review response dicts
- `borealis/tests/test_api.py` — update assertions; remove `attempts` from task frontmatter fixtures

## Todo

- [x] 1. Update `models.py`
- [x] 2. Update `parser.py`
- [x] 3. Update `writer.py`
- [x] 4. Update `api/features.py` and `api/tasks.py`
- [x] 5. Update `main.py`
- [x] 6. Update `tests/test_api.py`, `test_parser.py`, `test_resolver.py`

## Change History

- [2026-06-09] Plan created
- [2026-06-09] All changes implemented, 46/46 tests passing

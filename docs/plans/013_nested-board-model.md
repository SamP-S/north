# 013 — Nested Board Model

## Summary

Refactor `BoardState` from flat dicts (`tasks["project/feature/id"]`) to a nested tree
(`projects["name"].features["id"].tasks["id"]`) that mirrors the board's directory layout
and the API's URL structure. Drop `TaskModel.feature` (redundant — implicit from
containment) along with the `project`/`branch` properties on `TaskModel` and `branch`
property on `FeatureModel` (now derivable trivially from containment + `feature_id`).

State is loaded fully at startup and fully reloaded whenever a new commit is detected
on the board repo (no incremental diff-based upserts).

## New Model Shape

```python
@dataclass
class TaskModel:
    task_id: str
    title: str
    status: TaskStatus
    pipeline: str
    task_path: Path
    depends_on: list[str] = field(default_factory=list)
    created_at: datetime | None = None
    ready_at: datetime | None = None
    body: str = ""

@dataclass
class FeatureModel:
    feature_id: str
    title: str
    status: FeatureStatus
    feature_path: Path
    description: str = ""
    depends_on: list[str] = field(default_factory=list)
    created_at: datetime | None = None
    merged_at: datetime | None = None
    tasks: dict[str, TaskModel] = field(default_factory=dict)

    @property
    def branch(self) -> str:
        return self.feature_id

@dataclass
class ProjectModel:
    name: str
    ssh_url: str
    epics: dict[str, EpicModel] = field(default_factory=dict)
    features: dict[str, FeatureModel] = field(default_factory=dict)

@dataclass
class BoardState:
    projects: dict[str, ProjectModel] = field(default_factory=dict)
```

`EpicModel` unchanged (drop its `project` field — implicit via containment).

## Files to Modify

- `borealis/service/models.py` — nested dataclasses
- `borealis/service/board/parser.py` — parsers no longer set `project`/`feature` cross-refs
- `borealis/service/board/loader.py` — build nested tree directly
- `borealis/service/orchestrator/resolver.py` — iterate nested tree
- `borealis/service/orchestrator/git_watcher.py` — full reload on new commit instead of diff-based upsert
- `borealis/service/orchestrator/supervisor.py` — call full reload
- `borealis/service/api/tasks.py` — traversal-based lookups
- `borealis/service/api/features.py` — traversal-based lookups
- `borealis/service/main.py` — traversal-based lookups
- `borealis/tests/test_parser.py`, `test_resolver.py`, `test_api.py` — update for new model

## Todo

- [x] 1. Rewrite `models.py` with nested dataclasses
- [x] 2. Update `parser.py` (drop cross-ref fields)
- [x] 3. Rewrite `loader.py` to build nested tree
- [x] 4. Rewrite `resolver.py` for nested traversal
- [x] 5. Rewrite `git_watcher.py` to do full reload on new commit; update `supervisor.py`
- [x] 6. Update `api/tasks.py`, `api/features.py`, `main.py` for traversal lookups
- [x] 7. Update all three test files
- [x] 8. Run full test suite + ruff, fix issues

## Change History

- [2026-06-09] Plan created
- [2026-06-09] All changes implemented across models, parser, loader, resolver,
  git_watcher, supervisor, api/tasks.py, api/features.py, main.py, and all three
  test files. `deps.py` updated to take a `Callable[[], BoardState]` getter so
  full-reload replacement is reflected. 48/48 tests passing, ruff clean (excluding
  pre-existing unrelated `startup.py:21` E501).
- [2026-06-10] Dropped epics from Borealis entirely: removed `EpicModel`,
  `EpicStatus`, `ProjectModel.epics`, `parse_epic`, and `_load_epics`. Updated
  `docs/design/99_planned-features.md` to drop the epic reference in the
  project/feature/task timeline line. 48/48 tests passing, ruff clean.

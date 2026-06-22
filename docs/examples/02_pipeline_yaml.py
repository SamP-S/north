"""
Aurora pipeline example — YAML definition compiled to LangGraph.

Shows the PipelineCompiler: reads Aurora's definitions/pipelines/*.yaml format
(docs/design/15_pipelines.md §15.2-15.9) and builds a LangGraph StateGraph
from it at startup. At task pickup, select a pipeline by name and invoke it.

The YAML format is exactly Aurora's native format — no translation layer.
The compiler is what Aurora's run_pipeline() would call instead of its
current custom step-execution loop.

Run: uv run python docs/examples/02_pipeline_yaml.py
Requires: pip install langgraph langchain-core pyyaml
"""

from __future__ import annotations

import asyncio
import operator
from dataclasses import dataclass
from typing import Annotated, TypedDict

import yaml
from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, START, StateGraph

# ── Pipeline YAML ──────────────────────────────────────────────────────────────
# In production this lives at definitions/pipelines/map-code-review.yaml.
# Reproduced verbatim from docs/design/15_pipelines.md §15.9.

PIPELINE_YAML = """
name: map-code-review
entry: mapper
steps:
  - id: mapper
    agent: basic_mapper
    max_attempts: 2
    confidence:
      high: coder
      medium: coder
      low: coder
      blocked: stop
    on_fail: stop

  - id: coder
    agent: basic_coder
    max_attempts: 3
    confidence:
      high: qa_checker
      medium: qa_checker
      low: qa_checker
      blocked: stop
    on_fail: stop

  - id: qa_checker
    agent: qa_checker
    max_attempts: 1
    confidence:
      high: reviewer
      medium: sonnet_coder
      low: sonnet_coder
      blocked: stop
    on_fail: stop

  - id: sonnet_coder
    agent: sonnet_coder
    max_attempts: 2
    confidence:
      high: qa_checker
      medium: qa_checker
      low: stop
      blocked: stop
    on_fail: stop

  - id: reviewer
    agent: reviewer
    max_attempts: 1
    confidence:
      high: done
      medium: done
      low: stop
      blocked: stop
    on_fail: stop
"""


# ── Data model (shared with 01_pipeline_python.py) ────────────────────────────


class Artifact(TypedDict):
    agent: str
    confidence: str  # high | medium | low | blocked
    status: str      # complete | failed | blocked
    summary: str
    body: str


class PipelineState(TypedDict):
    task_id: str
    artifacts: Annotated[list[Artifact], operator.add]
    step_attempts: dict[str, int]
    final_status: str


# ── Agent definitions ──────────────────────────────────────────────────────────
# In production these are loaded from definitions/agents/*.md + board repo overrides.
# Aurora's provider inference (§5.6): claude-* | opus | sonnet | haiku → cloud;
# everything else → local.

_CLOUD_PREFIXES = ("claude-", "opus", "sonnet", "haiku")


@dataclass
class AgentDef:
    name: str
    model: str       # e.g. "claude-sonnet-4-6" or "mistral:7b-q4"
    max_turns: int = 20
    max_budget_usd: float = 0.50


# Minimal roster — mirrors what agent_prepare() would build
AGENT_ROSTER: dict[str, AgentDef] = {
    "basic_mapper":  AgentDef("basic_mapper",  model="mistral:7b-q4"),
    "basic_coder":   AgentDef("basic_coder",   model="claude-haiku-4-5-20251001"),
    "qa_checker":    AgentDef("qa_checker",    model="claude-haiku-4-5-20251001"),
    "sonnet_coder":  AgentDef("sonnet_coder",  model="claude-sonnet-4-6"),
    "reviewer":      AgentDef("reviewer",      model="claude-sonnet-4-6"),
}


def _is_cloud(model: str) -> bool:
    return any(model.startswith(p) or model == p for p in _CLOUD_PREFIXES)


# ── Stub executors ─────────────────────────────────────────────────────────────


async def _run_local(agent_def: AgentDef, prior_artifacts: list[Artifact]) -> Artifact:
    """Stub — Ollama /api/chat multi-turn tool execution loop (§6.5)."""
    print(f"  [local:{agent_def.model}]  {agent_def.name}")
    return Artifact(
        agent=agent_def.name, confidence="high", status="complete",
        summary=f"{agent_def.name}: plan produced by local model",
        body=f"[{agent_def.name} local output]",
    )


async def _run_cloud(agent_def: AgentDef, prior_artifacts: list[Artifact]) -> Artifact:
    """Stub — claude_agent_sdk.query() with agent definition (§6.1)."""
    print(f"  [cloud:{agent_def.model}]  {agent_def.name}")
    return Artifact(
        agent=agent_def.name, confidence="high", status="complete",
        summary=f"{agent_def.name}: task complete",
        body=f"[{agent_def.name} cloud output]",
    )


# ── Terminal nodes (same semantics as 01_pipeline_python.py) ──────────────────


async def done_node(state: PipelineState) -> dict:
    return {"final_status": "done"}


async def stop_node(state: PipelineState) -> dict:
    return {"final_status": "failed"}


# ── Pipeline compiler ──────────────────────────────────────────────────────────


class PipelineCompiler:
    """
    Compiles an Aurora pipeline YAML definition into a LangGraph CompiledStateGraph.

    Usage:
        compiler = PipelineCompiler(agent_roster=AGENT_ROSTER)
        graph    = compiler.compile(yaml_string=PIPELINE_YAML)
        result   = await graph.ainvoke(initial_state, config=config)
    """

    def __init__(self, agent_roster: dict[str, AgentDef]) -> None:
        self._roster = agent_roster

    def compile_from_file(self, path: str):
        with open(path) as f:
            return self.compile(f.read())

    def compile(self, yaml_string: str):
        config = yaml.safe_load(yaml_string)
        self._validate(config)

        graph = StateGraph(PipelineState)
        step_ids = {s["id"] for s in config["steps"]}
        all_targets = step_ids | {"done", "stop"}

        # Step nodes — one per pipeline step
        for step in config["steps"]:
            graph.add_node(step["id"], self._make_node(step))

        # Terminal nodes
        graph.add_node("done", done_node)
        graph.add_node("stop", stop_node)
        graph.add_edge("done", END)
        graph.add_edge("stop", END)

        # Entry
        graph.add_edge(START, config["entry"])

        # Conditional edges — driven entirely by the YAML routing tables
        for step in config["steps"]:
            graph.add_conditional_edges(
                step["id"],
                self._make_router(step),
                all_targets,
            )

        return graph.compile(checkpointer=MemorySaver())

    # ── Node factory ──────────────────────────────────────────────────────────

    def _make_node(self, step: dict):
        """Returns an async node function for the given pipeline step."""
        step_id = step["id"]
        agent_name = step["agent"]

        async def node(state: PipelineState) -> dict:
            agent_def = self._roster.get(agent_name)
            if agent_def is None:
                # Missing agent definition → blocked (§6.2 agent_prepare failure)
                return {
                    "artifacts": [Artifact(
                        agent=agent_name, confidence="blocked", status="blocked",
                        summary=f"agent_prepare failed: no definition for '{agent_name}'",
                        body="",
                    )],
                    "step_attempts": {
                        **state["step_attempts"],
                        step_id: state["step_attempts"].get(step_id, 0) + 1,
                    },
                }

            artifact = (
                await _run_cloud(agent_def, state["artifacts"])
                if _is_cloud(agent_def.model)
                else await _run_local(agent_def, state["artifacts"])
            )
            return {
                "artifacts": [artifact],
                "step_attempts": {
                    **state["step_attempts"],
                    step_id: state["step_attempts"].get(step_id, 0) + 1,
                },
            }

        node.__name__ = step_id
        return node

    # ── Router factory ────────────────────────────────────────────────────────

    def _make_router(self, step: dict):
        """
        Returns the conditional-edge router function for a given step config.

        Reads confidence from the last artifact.
        On malformed artifact: retries up to max_attempts, then follows on_fail.
        On valid confidence: looks up the routing table from the YAML.
        """
        step_id = step["id"]
        routing: dict[str, str] = step["confidence"]
        max_attempts: int = step.get("max_attempts", 1)
        on_fail: str = step["on_fail"]

        def router(state: PipelineState) -> str:
            last = state["artifacts"][-1]
            attempts = state["step_attempts"].get(step_id, 0)

            if last["confidence"] not in ("high", "medium", "low", "blocked"):
                # Malformed artifact — retry or on_fail
                return step_id if attempts < max_attempts else on_fail

            return routing[last["confidence"]]

        return router

    # ── Validation ────────────────────────────────────────────────────────────

    @staticmethod
    def _validate(config: dict) -> None:
        """
        Validates a parsed pipeline config. Mirrors the pipeline loader
        described in docs/design/15_pipelines.md §15.2.

        Raises ValueError with a clear message if validation fails — the caller
        should set task status: blocked.
        """
        required_top = ("name", "entry", "steps")
        for field in required_top:
            if field not in config:
                raise ValueError(f"Pipeline missing required field: '{field}'")

        step_ids = {s["id"] for s in config["steps"]}
        valid_targets = step_ids | {"done", "stop"}

        if config["entry"] not in step_ids:
            raise ValueError(f"entry '{config['entry']}' does not reference a valid step id")

        required_confidence = ("high", "medium", "low", "blocked")

        for step in config["steps"]:
            sid = step.get("id", "<missing>")
            for field in ("id", "agent", "confidence", "on_fail"):
                if field not in step:
                    raise ValueError(f"Step '{sid}' missing required field: '{field}'")

            for level in required_confidence:
                if level not in step["confidence"]:
                    raise ValueError(f"Step '{sid}' confidence missing level: '{level}'")
                target = step["confidence"][level]
                if target not in valid_targets:
                    raise ValueError(
                        f"Step '{sid}' confidence.{level} target '{target}' "
                        f"is not a valid step id, 'done', or 'stop'"
                    )

            if step["on_fail"] not in valid_targets:
                raise ValueError(
                    f"Step '{sid}' on_fail target '{step['on_fail']}' "
                    f"is not a valid step id, 'done', or 'stop'"
                )


# ── Entry point ────────────────────────────────────────────────────────────────


async def main() -> None:
    compiler = PipelineCompiler(agent_roster=AGENT_ROSTER)

    # Compile all pipelines at service startup (cache by name in production)
    pipeline = compiler.compile(yaml_string=PIPELINE_YAML)

    initial_state: PipelineState = {
        "task_id": "aurora-002",
        "artifacts": [
            {
                "agent": "system",
                "confidence": "high",
                "status": "complete",
                "summary": "Task 002: Add JWT authentication",
                "body": (
                    "# Task 002 — Add JWT Authentication\n\n"
                    "Implement JWT-based auth in `src/auth.py` using PyJWT. "
                    "Add `POST /auth/token` endpoint. Store tokens in Redis with "
                    "a 1-hour TTL. All existing tests must still pass."
                ),
            }
        ],
        "step_attempts": {},
        "final_status": "",
    }

    config = {"configurable": {"thread_id": "aurora-002"}}

    print("Running map-code-review pipeline (compiled from YAML)...\n")
    result = await pipeline.ainvoke(initial_state, config=config)

    print(f"\nFinal status  : {result['final_status']}")
    print(f"Steps run     : {list(result['step_attempts'].keys())}")
    print(f"Artifact chain: {len(result['artifacts'])} artifacts\n")
    for i, a in enumerate(result["artifacts"]):
        print(f"  [{i}] agent={a['agent']:20s} confidence={a['confidence']}  {a['summary']}")


if __name__ == "__main__":
    asyncio.run(main())

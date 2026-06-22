"""
Aurora pipeline example — native Python LangGraph definition.

Implements the map-code-review pipeline (see docs/design/15_pipelines.md §15.9)
directly as a LangGraph StateGraph. Shows:

  - Artifact chain accumulation via Annotated state
  - Per-step node functions (stub local Ollama + cloud Claude executors)
  - Confidence-based routing via conditional edges
  - Retry loop (malformed artifact cycles back to same node)
  - on_fail routing when max_attempts exhausted
  - Separate done/stop terminal nodes preserving final_status

Run: uv run python docs/examples/01_pipeline_python.py
Requires: pip install langgraph langchain-core
"""

from __future__ import annotations

import asyncio
import operator
from typing import Annotated, TypedDict

from langgraph.checkpoint.memory import MemorySaver
from langgraph.graph import END, START, StateGraph


# ── Data model ────────────────────────────────────────────────────────────────


class Artifact(TypedDict):
    agent: str
    confidence: str  # high | medium | low | blocked
    status: str      # complete | failed | blocked
    summary: str
    body: str


class PipelineState(TypedDict):
    task_id: str
    artifacts: Annotated[list[Artifact], operator.add]  # append-only across nodes
    step_attempts: dict[str, int]                        # {step_id: attempt_count}
    final_status: str                                    # done | failed (set by terminal node)


# ── Stub executors ─────────────────────────────────────────────────────────────
# In production: replace with local_executor.complete() and claude_agent_sdk.query()


async def _run_local(agent: str, prior_artifacts: list[Artifact]) -> Artifact:
    """Stub — calls Ollama /api/chat with agent role prompt + prior artifact chain."""
    print(f"  [local]  {agent}")
    return Artifact(
        agent=agent,
        confidence="high",
        status="complete",
        summary=f"{agent}: mapped task to implementation plan",
        body=f"[{agent} output — plan produced by local 7B model]",
    )


async def _run_cloud(agent: str, prior_artifacts: list[Artifact]) -> Artifact:
    """Stub — calls claude_agent_sdk.query() with agent definition + prior artifact chain."""
    print(f"  [cloud]  {agent}")
    return Artifact(
        agent=agent,
        confidence="high",
        status="complete",
        summary=f"{agent}: task complete",
        body=f"[{agent} output — work produced by Claude cloud model]",
    )


# ── Step nodes ─────────────────────────────────────────────────────────────────
# Each node calls its executor, appends an artifact, and increments its attempt count.
# LangGraph merges {"artifacts": [new]} by appending (operator.add on the list).
# step_attempts replaces the whole dict — use spread to preserve other keys.


async def mapper(state: PipelineState) -> dict:
    artifact = await _run_local("basic_mapper", state["artifacts"])
    return {
        "artifacts": [artifact],
        "step_attempts": {**state["step_attempts"], "mapper": state["step_attempts"].get("mapper", 0) + 1},
    }


async def coder(state: PipelineState) -> dict:
    artifact = await _run_cloud("basic_coder", state["artifacts"])
    return {
        "artifacts": [artifact],
        "step_attempts": {**state["step_attempts"], "coder": state["step_attempts"].get("coder", 0) + 1},
    }


async def qa_checker(state: PipelineState) -> dict:
    artifact = await _run_cloud("qa_checker", state["artifacts"])
    return {
        "artifacts": [artifact],
        "step_attempts": {**state["step_attempts"], "qa_checker": state["step_attempts"].get("qa_checker", 0) + 1},
    }


async def sonnet_coder(state: PipelineState) -> dict:
    artifact = await _run_cloud("sonnet_coder", state["artifacts"])
    return {
        "artifacts": [artifact],
        "step_attempts": {**state["step_attempts"], "sonnet_coder": state["step_attempts"].get("sonnet_coder", 0) + 1},
    }


async def reviewer(state: PipelineState) -> dict:
    artifact = await _run_cloud("reviewer", state["artifacts"])
    return {
        "artifacts": [artifact],
        "step_attempts": {**state["step_attempts"], "reviewer": state["step_attempts"].get("reviewer", 0) + 1},
    }


# ── Terminal nodes ─────────────────────────────────────────────────────────────
# "done" and "stop" are routing targets in Aurora's pipeline YAML (§15.5).
# Both lead to END but record the final outcome in state first.


async def done_node(state: PipelineState) -> dict:
    return {"final_status": "done"}


async def stop_node(state: PipelineState) -> dict:
    return {"final_status": "failed"}


# ── Routing tables ─────────────────────────────────────────────────────────────
# Mirrors the confidence: blocks in definitions/pipelines/map-code-review.yaml (§15.9).

_MAX_ATTEMPTS: dict[str, int] = {
    "mapper": 2,
    "coder": 3,
    "qa_checker": 1,
    "sonnet_coder": 2,
    "reviewer": 1,
}

_ROUTING: dict[str, dict[str, str]] = {
    "mapper": {
        "high": "coder", "medium": "coder", "low": "coder", "blocked": "stop",
    },
    "coder": {
        "high": "qa_checker", "medium": "qa_checker", "low": "qa_checker", "blocked": "stop",
    },
    "qa_checker": {
        "high": "reviewer", "medium": "sonnet_coder", "low": "sonnet_coder", "blocked": "stop",
    },
    "sonnet_coder": {
        "high": "qa_checker", "medium": "qa_checker", "low": "stop", "blocked": "stop",
    },
    "reviewer": {
        "high": "done", "medium": "done", "low": "stop", "blocked": "stop",
    },
}


# ── Router factory ─────────────────────────────────────────────────────────────


def make_router(step_id: str):
    """
    Returns the conditional-edge router function for a given step.

    On a malformed artifact (missing/invalid confidence field) the step retries
    itself up to max_attempts times, then follows on_fail (always "stop" here).
    On a valid confidence value the routing table determines the next node.
    """
    def router(state: PipelineState) -> str:
        last = state["artifacts"][-1]
        attempts = state["step_attempts"].get(step_id, 0)

        if last["confidence"] not in ("high", "medium", "low", "blocked"):
            # Malformed artifact — retry or fail
            return step_id if attempts < _MAX_ATTEMPTS[step_id] else "stop"

        return _ROUTING[step_id][last["confidence"]]

    return router


# ── Graph construction ─────────────────────────────────────────────────────────

_ALL_NODES = {"mapper", "coder", "qa_checker", "sonnet_coder", "reviewer", "done", "stop"}


def build_pipeline():
    graph = StateGraph(PipelineState)

    # Step nodes
    for name, fn in [
        ("mapper", mapper),
        ("coder", coder),
        ("qa_checker", qa_checker),
        ("sonnet_coder", sonnet_coder),
        ("reviewer", reviewer),
    ]:
        graph.add_node(name, fn)

    # Terminal nodes
    graph.add_node("done", done_node)
    graph.add_node("stop", stop_node)
    graph.add_edge("done", END)
    graph.add_edge("stop", END)

    # Entry point
    graph.add_edge(START, "mapper")

    # Conditional edges — one per step, router decides next node at runtime
    for step_id in ("mapper", "coder", "qa_checker", "sonnet_coder", "reviewer"):
        graph.add_conditional_edges(step_id, make_router(step_id), _ALL_NODES)

    # Checkpointer enables state persistence, time-travel, and human-in-the-loop
    return graph.compile(checkpointer=MemorySaver())


# ── Entry point ────────────────────────────────────────────────────────────────


async def main() -> None:
    pipeline = build_pipeline()

    # task_ingest produces artifact[0] — this seeds the chain (§5.2, §15.6)
    initial_state: PipelineState = {
        "task_id": "aurora-001",
        "artifacts": [
            {
                "agent": "system",
                "confidence": "high",
                "status": "complete",
                "summary": "Task 001: Add JWT authentication",
                "body": (
                    "# Task 001 — Add JWT Authentication\n\n"
                    "Implement JWT-based auth in `src/auth.py` using PyJWT. "
                    "Add `POST /auth/token` endpoint. Store tokens in Redis with "
                    "a 1-hour TTL. All existing tests must still pass."
                ),
            }
        ],
        "step_attempts": {},
        "final_status": "",
    }

    config = {"configurable": {"thread_id": "aurora-001"}}

    print("Running map-code-review pipeline...\n")
    result = await pipeline.ainvoke(initial_state, config=config)

    print(f"\nFinal status  : {result['final_status']}")
    print(f"Steps run     : {list(result['step_attempts'].keys())}")
    print(f"Artifact chain: {len(result['artifacts'])} artifacts\n")
    for i, a in enumerate(result["artifacts"]):
        print(f"  [{i}] agent={a['agent']:20s} confidence={a['confidence']}  {a['summary']}")


if __name__ == "__main__":
    asyncio.run(main())

# North v2 Architecture

## Goals

- Keep North focused on its differentiated value: the git-backed board, the
  workflow automation, and the feature/review lifecycle.
- Use existing tooling for agent execution instead of maintaining our own
  provider integrations (Claude Agent SDK wiring, Ollama tool-calling loop).
- Make the agent runtime swappable: a small adapter interface owned by Aurora,
  with opencode as the primary implementation.

## System Overview

```
┌────────────┐   REST    ┌────────────┐   AgentRuntime    ┌──────────────────┐
│  Borealis  │◄─────────►│   Aurora   │──── adapter ─────►│  opencode serve  │
│  (board)   │           │ (workflow) │                   │ (agent runtime)  │
└────────────┘           └────────────┘                   └──────────────────┘
      │                        │                                  │
  board repo             worktrees, git                 providers: Anthropic,
  (markdown)             merge/review ops               Ollama; tools; MCP
```

### Borealis — board state manager (unchanged)

Owns the git-backed task board, exposes the REST API for projects, features,
tasks, queue, and review. No knowledge of agents or pipelines beyond the
`pipeline` field on tasks.

### Aurora — workflow automator

- Polls Borealis for eligible tasks; marks them in progress.
- Owns all git operations: managed clones, feature branches, worktrees,
  approve/rollback/reject.
- Loads pipeline definitions (YAML step graphs with confidence-based routing)
  and agent definitions (markdown frontmatter), and walks the pipeline graph.
- Executes each step by calling the agent runtime through the `AgentRuntime`
  adapter interface, passing the worktree path explicitly.
- Enforces the artifact contract: each step must return Markdown with
  `agent/confidence/status/summary` frontmatter; routing uses `confidence`.
  Artifact parsing stays in Aurora — runtimes return raw text.

### Agent runtime — opencode serve

A headless HTTP service (`opencode serve`) that handles provider access
(Anthropic, Ollama), sessions, tool use, and permissions. Reached only through
the adapter. Chosen because it covers the gateway role we would otherwise
build: explicit per-directory sessions, sync and async messaging
(`prompt_async`), an SSE event bus, an OpenAPI spec and SDKs, and standalone
CLI/TUI usability.

## The AgentRuntime adapter

Aurora-internal interface (Python `Protocol`); implementations are
constructor-injected into the pipeline executor.

```python
class AgentRuntime(Protocol):
    async def run_step(self, request: StepRequest) -> StepResult: ...

@dataclass
class StepRequest:
    agent: AgentDefinition       # role prompt, model, tools, limits
    artifacts: list[Artifact]    # prior pipeline artifacts (user prompt)
    workdir: Path                # worktree — always explicit
    timeout_s: int

@dataclass
class StepResult:
    text: str                    # raw agent output (artifact parsed by Aurora)
    outcome: Outcome             # ok | rate_limited | auth_failed | timeout | error
    error: str = ""
```

Implementations:

- `OpencodeRuntime` — primary. Creates a session against `workdir`, sends the
  prompt, collects the final message. v1 uses the synchronous message endpoint
  inside Aurora's existing per-step timeout; migrate to `prompt_async` + SSE
  once step durations or concurrency demand it.
- `LegacyRuntime` — wraps the existing `cloud.py`/`local.py` paths during the
  transition; deleted once opencode is proven.

Runtime selection is a config setting (`agent_runtime: opencode | legacy`).

## Deployment

Three systemd user units: `borealis.service`, `aurora.service`,
`opencode.service` (replacing direct SDK/Ollama wiring; Ollama remains as a
backend used by opencode). All on one host sharing `~/.north`; worktree paths
are passed by absolute path, which bakes in same-host deployment by design.

## Planned capabilities and where they live

- **Memory / RAG** — implemented as MCP servers, consumed by opencode (and
  usable from any other MCP client, e.g. Claude Code). Not baked into Aurora.
- **Live updates (SSE)** — Borealis re-adds its event stream per
  `99_planned-features.md`; Aurora may bridge opencode events into it.
- **Concurrency** — `runner_concurrency > 1` becomes feasible once steps run
  in opencode sessions; out of scope for the initial migration.

## Risks

- opencode is fast-moving; some endpoints are experimental. Mitigation: all
  access goes through the adapter; pin the opencode version in install.sh.
- Artifact contract is prompt-enforced; smaller local models may fail to emit
  valid frontmatter. Mitigation: existing `max_attempts` retry handles this;
  spike validates Ollama behaviour before cutover.
- Permission model differs from the current bash denylist. Mitigation: spike
  must confirm opencode's permission config can express deny rules equivalent
  to `GLOBAL_BASH_DENYLIST`.

## Migration plans

1. `docs/plans/015_borealis-integration-fixes.md` — fix the Aurora↔Borealis
   contract first (prerequisite).
2. `docs/plans/016_borealis-improvements.md` — Borealis CLI/API repair and
   write commands (independent of the runtime work).
3. `docs/plans/017_agent-runtime-adapter.md` — carve the adapter seam inside
   Aurora; legacy execution behind it.
4. `docs/plans/018_opencode-runtime.md` — spike, implement `OpencodeRuntime`,
   deploy, cut over, delete legacy.

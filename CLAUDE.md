# CRITICAL RULES - MUST FOLLOW

This is project "North", a Go program providing an **in-repo Markdown task board** (modeled on Backlog.md): a `north/` directory committed inside the user's project repo, where each task is a plain Markdown file and its status is the folder it lives in. It ships a single `north` binary (CLI) and an optional on-demand MCP server. There is no daemon and no REST API; git is the user's responsibility (North never pushes/pulls). It builds to one static binary with no runtime dependency. The only installable package is `cmd/north`; all library code lives under `internal/` (board logic in `internal/board` + `internal/tasks`, the MCP server in `internal/service`, the CLI in `internal/cli`).

## RESPONSES
- Keep responses concise and to the point - unless the user asks otherwise

## CORE RULES
- Always ask if anything is unclear, incomplete, or has possible issues.
- Keep answers and questions short and concise; further detail should be provided if requested by the user.
- English only: all code, comments, documentation, and communication must be in English.

## SUGGESTING CHANGES
- Show relevant file content before editing.
- Explain reasoning before making changes.

## PLANNING MODE
- Always ask clarifying questions
- Never assume design, tech stack, packages, libraries or features
- Use deep-dive sub-agents to assist with research
- Use deep-dive sub-agents to review the different aspects of your plan before presenting to the user
- Always write plans before implementing any code changes.
- Store plans in "docs/plans/"
- Sequentially number all plans as a prefix and do not use spaces in filenames, i.e. 003_impl-aabb-collision
- Continually update plan as changes are made. Plan and changes should always be in sync

## CHANGE / EDIT MODE
- Never implement features yourself when possible - use sub-agents!
- Identify changes from the plan that can be implemented in parallel, and use sub-agents to implement the features efficiently
- When using sub-agents to implement features, act as a coordinator only
- Use the best model for the task - premium models for complex tasks (like coding) and mid-tier models for simpler tasks, like documentation
- After completing features (large or small), always run commands like lint, type check and next build to check code quality
- Plans must include:
  - Summary of features/fixes/scope
  - Files to modify
  - Ordered numerical todo list (checkbox trackable progress)
  - Change history (update as changes made with timestamps i.e. [2026-05-09])

## TESTING
- Use any testing tools, libraries available to the project for testing your changes
- Never assume your changes simply work, always test!
- If the project does not have any testing tools, scripts, MCP tools, skills, etc. available for testing, ask the user whether testing should be skipped.

## CODE STYLE
- Go 1.25+, doc comments on exported identifiers (start with the identifier name).
- Naming: Go conventions — `MixedCaps` exported, `mixedCaps` unexported; short, idiomatic names; package names lower-case, no underscores.
- Errors: return `error`; domain failures use `internal/errors` (`NotFound`/`Conflict`/`Invalid`).
- Minimize imports; prioritize the standard library over third-party packages.
- Use `go build` / `go test` for builds and tests; `go mod` for dependencies.
- Format with `gofmt`; check with `go vet` (see the `Makefile`: `make build|test|vet|fmt`).

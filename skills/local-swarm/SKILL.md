---
name: local-swarm
description: Delegate mechanical, low-judgment work (boilerplate, log or diff summarization, formatting, repetitive transforms, first-draft docs) to local or self-hosted models through the local-swarm-mcp MCP server, to save the main model's tokens. Use when the local-swarm-mcp MCP tools (delegate_task, spawn_task, ...) are available and a task is mechanical enough that a smaller model can do it and the result is cheap to check; also when the user asks to "use the local model", "delegate", "offload", or "save tokens". Do not use for architecture decisions, security-sensitive work, or anything whose correctness can't be verified quickly.
license: MIT
compatibility: Requires the local-swarm-mcp MCP server registered in the agent (any MCP client - Claude Code, Codex, Cursor, GitHub Copilot, opencode, Gemini CLI).
---

# Delegating to local models with local-swarm-mcp

local-swarm-mcp runs tasks on models the user hosts (Ollama, llama.cpp,
vLLM, or any OpenAI-compatible endpoint). Those models are cheaper and
weaker than you: delegate work that is **mechanical** and **cheap to
verify**, and keep judgment for yourself. The server is a cost
optimization, never a hard dependency - when it isn't usable, do the work
yourself.

Tool names below are the bare MCP names. Each agent shows them with its
own prefix for the server (e.g. `mcp__local-swarm-mcp__delegate_task` in
Claude Code); match on the part after the server name.

## 1. Check availability first

1. `list_backends` - the first entry is the default used when you omit
   `backend`. Empty list → no model is configured; skip delegation.
2. `health_check` (optionally with `backend`) - confirms the endpoint
   answers right now. A discovered host that went offline drops out of
   `list_backends` on its next poll; `list_backend_hosts` shows `up`.
3. If nothing is healthy, **fall back to doing the work yourself**. Don't
   retry in a loop or ask the user to start their GPU box unless they
   asked you to use the local model specifically.

## 2. Decide whether to delegate

Delegate when all of these hold:

- The task is mechanical: transform, summarize, extract, reformat,
  generate boilerplate from a clear spec.
- You can check the output faster than you could write it.
- The input contains no secrets, credentials, or data the user wouldn't
  send to that backend (it may be a remote host - check `base_url`).

`classify_task_risk` is a fast keyword check (destructive git/DB
operations, secrets, "architecture", "production", "security"). A
`risk=high` result means do it yourself; `risk=low` is **not** a
guarantee - still apply the rules above.

## 3. Pick the right tool

| Situation | Tool |
|---|---|
| One short request, you need the answer now | `delegate_task` |
| Long-running or several independent pieces | `spawn_task` per piece, then `wait_task` (or `task_status`) |
| Iterative work where later turns need earlier context | `create_session` → `send_message` … → `close_session` |
| A large blob is filling your context | `estimate_tokens`, then `compact_context` with `preserve_instructions` |
| Stash a big intermediate result outside your context | `scratch_set` / `scratch_get` / `scratch_list` / `scratch_delete` |
| The model must call tools itself (e.g. query a code index) | `spawn_agent_task` - see [references/tools.md](references/tools.md) |

Prompting a small model well:

- Put the whole task in `prompt`, self-contained: it sees nothing of your
  conversation. Use `system_prompt` for the output contract ("Reply with
  only the JSON, no prose").
- Use `temperature: 0` for transforms and extraction (it is honored, not
  dropped); keep the default 0.2 for prose.
- Set `max_tokens` to what the answer needs; the default is 1024.
- Ask for one thing per call. Split big jobs into several `spawn_task`s
  rather than one giant prompt.

## 4. Always verify the result

Treat delegated output like a junior's first draft:

- Read it before using it. Check code compiles or tests pass, JSON
  parses, numbers and paths match the source.
- For `spawn_agent_task`, read the `transcript` in the final
  `task_status` / `wait_task` result to see which tools it actually
  called; don't trust a claim the transcript doesn't support.
- If the output is wrong twice, stop delegating that task and do it
  yourself.
- Tell the user when a result came from the local model, if it matters
  to them.

## 5. Limits worth knowing

- `wait_task` returns an error with a `snapshot=` of the task when its
  `timeout_seconds` (default 120) passes; the task keeps running - wait
  again or `cancel_task` it.
- Finished tasks are dropped after an hour (at most 500 kept): fetch
  results you need promptly.
- Sessions keep only their last 40 messages.
- Requests queue when a backend host already has its maximum in flight
  (default 4), so many parallel `spawn_task`s finish over time, not at once.
- `scratch_*` may answer "held by another local-swarm-mcp process" when
  another session or the daemon owns the store; it retries on every call.
  Don't treat that as data loss.

## Never

- Never put secrets, tokens, or `.env` contents in a prompt.
- Never call `register_downstream_mcp_server` (when offered at all) or
  `register_backend_host` unless the user asked for that specific server
  or host: both make this server run commands or send data somewhere new.
- Never point `spawn_agent_task` at downstream servers with destructive
  tools unless the user explicitly wants that; restrict it with
  `mcp_servers`.

For per-tool parameters, model capability notes, and troubleshooting, see
[references/tools.md](references/tools.md),
[references/models.md](references/models.md), and
[references/troubleshooting.md](references/troubleshooting.md).

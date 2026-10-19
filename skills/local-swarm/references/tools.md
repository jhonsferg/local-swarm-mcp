# local-swarm-mcp tool reference

Every tool that takes `backend` uses the default when it's omitted: the
first statically configured backend, or - with none - the first
discovered `<host>/<model>` backend in name order (the first entry of
`list_backends`). Errors come back as tool results with `isError: true`
and a plain-text message.

## Backends and hosts

| Tool | Parameters | Returns |
|---|---|---|
| `list_backends` | - | `[{name, base_url, model, has_api_key}]`, default first |
| `health_check` | `backend?` | `[{name, healthy, detail?}]` (probes `GET <base_url>/models`) |
| `list_backend_hosts` | - | `[{name, base_url, has_api_key, models, up, last_seen, last_error?}]` |
| `register_backend_host` | `name`, `base_url` (Ollama root, no `/v1`), `api_key?` | text; models become `<name>/<model>` backends after an immediate poll. Re-registering with no `api_key` keeps the stored key |
| `unregister_backend_host` | `name` | text; its backends disappear immediately |

API keys are never returned by any tool.

## Delegation

| Tool | Parameters | Returns |
|---|---|---|
| `delegate_task` | `prompt`, `backend?`, `system_prompt?`, `max_tokens`=1024, `temperature`=0.2, `top_p?` | the reply text |
| `compact_context` | `text`, `target_tokens`=500, `preserve_instructions?`, `backend?` | a summary (runs at temperature 0.1, `max_tokens` = 2 × target) |
| `estimate_tokens` | `text` | `~N tokens` (chars/4 heuristic, no tokenizer) |
| `classify_task_risk` | `description` | `risk=high: reasons…` or `risk=low: …` (keyword match only) |

## Background tasks

| Tool | Parameters | Returns |
|---|---|---|
| `spawn_task` | same as `delegate_task` | task ID |
| `task_status` | `task_id` | `{id, backend, status, result?, error?, started_at, finished_at?, transcript?}` |
| `wait_task` | `task_id`, `timeout_seconds`=120 | final snapshot, or an error ending in `snapshot={…}` on timeout |
| `list_tasks` | - | all tracked tasks |
| `cancel_task` | `task_id` | `cancelled`, or an error if already finished |

`status` is one of `pending`, `running`, `completed`, `failed`,
`cancelled`.

## Tool-using agents

| Tool | Parameters | Returns |
|---|---|---|
| `list_available_agent_tools` | `mcp_servers?` | `[{name: "<server>__<tool>", server, description}]` |
| `spawn_agent_task` | `prompt`, `backend?`, `system_prompt?`, `mcp_servers?`, `max_iterations`=8, `max_tokens`=1024, `temperature`=0.2, `top_p?` | task ID; track with the task tools |

The agent loop offers the model the downstream tools allowed by
`mcp_servers` (all connected servers if omitted) and runs up to
`max_iterations` completion rounds. A call to any tool that wasn't
offered is refused and recorded with `is_error: true`. Each tool result
is truncated to 16 KiB before going back to the model. Hitting
`max_iterations` without a final answer makes the task `failed`.

## Sessions

| Tool | Parameters | Returns |
|---|---|---|
| `create_session` | `backend?`, `system_prompt?` | session ID |
| `send_message` | `session_id`, `message`, `max_tokens`=1024, `temperature`=0.2, `top_p?` | the reply |
| `session_history` | `session_id` | `[{role, content}]` |
| `list_sessions` | - | `[{id, backend, message_count}]` |
| `close_session` | `session_id` | `closed` |

## Scratch store

`scratch_set {key, value}`, `scratch_get {key}`, `scratch_list`,
`scratch_delete {key}` - a persistent key-value file shared by every
local-swarm-mcp process on the machine (one holder at a time; others get
a "held by another local-swarm-mcp process" error until it's free).

## Downstream MCP servers

`list_downstream_mcp_servers`, `unregister_downstream_mcp_server {name}`,
and - only when the server was started with
`-allow-command-registration` - `register_downstream_mcp_server {name,
command, args?}`. Names may not contain `__`.

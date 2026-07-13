# local-swarm-mcp

An MCP server that delegates mechanical, low-judgment tasks to local or
remote OpenAI-compatible inference backends (llama.cpp's `llama-server`,
Ollama, vLLM, or any hosted provider exposing the same API shape), and gives
an MCP client full control over that delegated work: fire-and-forget
background tasks, multi-turn sessions, a persistent scratch store, and
context-budgeting helpers - the same primitives an agent orchestration
system offers for its own subagents, backed by a model running on your own
hardware.

## Why

Judgment-heavy work (architecture decisions, deciding whether a finding is a
false positive, writing meaningful tests) needs a strong model. Mechanical
work (boilerplate generation, log summarization, formatting, repetitive
transforms) doesn't. This server lets an MCP client offload the latter to
whatever hardware you already have running a local model, keeping its own
token budget for the former - and treat that offloaded work like its own
background agents rather than a single blocking request/response call.

## Prerequisites

**1. An OpenAI-compatible inference backend, running somewhere reachable.**
local-swarm-mcp does not embed or bundle an inference engine itself - it's a
thin client in front of one. Pick whichever fits your hardware:

- **llama.cpp** (recommended for mixed NVIDIA/AMD hardware - its Vulkan
  backend runs on both without depending on ROCm/CUDA maturity):
  ```
  # build or download llama-server, then:
  llama-server -m /path/to/model.gguf --host 0.0.0.0 --port 8080
  ```
- **Ollama** (easiest to set up, auto-detects your GPU backend):
  ```
  ollama serve
  ollama pull qwen2.5-coder
  ```
  Ollama's OpenAI-compatible endpoint is at `http://localhost:11434/v1`.
- **vLLM**, or any hosted provider with an OpenAI-compatible
  `/v1/chat/completions` endpoint, also work - just point a backend entry
  at it.

**2. Go 1.26 or newer**, to build local-swarm-mcp itself:
```
go build -o local-swarm-mcp ./cmd/local-swarm-mcp
```
(No prebuilt release binaries yet - this is a v0.1 project. If you add a
release workflow later, update this section.)

## Configuring backends

Backends can come from a config file, from command-line flags, or both (an
ad-hoc `-backend-url` is appended on top of whatever the config file
loaded). At least one backend must end up configured or the server refuses
to start.

### Option A: config file (YAML or JSON)

Format is auto-detected from the file extension (`.json` => JSON, anything
else => YAML); override with `-config-format`.

`config.yaml`:
```yaml
backends:
  - name: local-llama
    base_url: http://localhost:8080/v1
    model: qwen2.5-coder

  - name: local-ollama
    base_url: http://localhost:11434/v1
    model: qwen2.5-coder:7b

# Optional - defaults to <user cache dir>/local-swarm-mcp/scratch.db
store_path: C:\Users\you\.cache\local-swarm-mcp\scratch.db
```

Equivalent `config.json`:
```json
{
  "backends": [
    { "name": "local-llama", "base_url": "http://localhost:8080/v1", "model": "qwen2.5-coder" }
  ],
  "store_path": "C:\\Users\\you\\.cache\\local-swarm-mcp\\scratch.db"
}
```

Run with `-config path/to/config.yaml` (or `.json`). If `-config` is
omitted, the server looks for `<user config dir>/local-swarm-mcp/config.yaml`
(`%APPDATA%\local-swarm-mcp\config.yaml` on Windows,
`~/.config/local-swarm-mcp/config.yaml` on Linux/macOS) - if that file
doesn't exist either, a missing config is not an error on its own, as long
as `-backend-url` supplies at least one backend.

### Option B: flags only, no config file

```
local-swarm-mcp \
  -backend-name local-llama \
  -backend-url http://localhost:8080/v1 \
  -backend-model qwen2.5-coder
```

### All flags

| Flag | Default | Purpose |
|---|---|---|
| `-config` | `<user config dir>/local-swarm-mcp/config.yaml` | Path to a YAML or JSON config file |
| `-config-format` | auto-detect from `-config`'s extension | Force `"yaml"` or `"json"` parsing |
| `-backend-name` | `cli` | Name for the ad-hoc backend given via `-backend-url` |
| `-backend-url` | *(none)* | Base URL for an ad-hoc backend, added on top of any config-file backends |
| `-backend-model` | *(none)* | Model name for the ad-hoc backend |
| `-backend-key` | *(none)* | API key for the ad-hoc backend, if any |
| `-store-path` | config file's `store_path`, else `<user cache dir>/local-swarm-mcp/scratch.db` | Override the scratch-store file location |

## Registering with an MCP client

Add an entry to your client's MCP config (e.g. Claude Code's `.mcp.json`)
pointing `command` at the built binary, with any flags you need in `args`:

```json
{
  "mcpServers": {
    "local-swarm-mcp": {
      "command": "/path/to/local-swarm-mcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

## Tools

### Backends
| Tool | Purpose |
|---|---|
| `list_backends` | List configured backends (name, base_url, model) |
| `health_check` | Probe reachability of one backend, or all if omitted |

### One-shot delegation
| Tool | Purpose |
|---|---|
| `delegate_task` | Send a task to a backend and block for the completion - the simple synchronous path |
| `compact_context` | Summarize a block of text down to a target size via a backend, so it doesn't sit uncompacted in the client's own context |

### Background tasks (fire-and-forget, like spawning a subagent)
| Tool | Purpose |
|---|---|
| `spawn_task` | Start a task in the background, return a task ID immediately |
| `task_status` | Non-blocking snapshot of a task's state (pending/running/completed/failed/cancelled) |
| `wait_task` | Block until a task finishes or a timeout elapses, then return its final snapshot |
| `list_tasks` | List every task spawned this server run |
| `cancel_task` | Cancel a still-running task |

### Sessions (persistent multi-turn conversations, like resuming a named agent)
| Tool | Purpose |
|---|---|
| `create_session` | Open a session against a backend with an optional system prompt |
| `send_message` | Send a message within a session, carrying its full prior history, and get the reply |
| `session_history` | Return a session's full message history |
| `close_session` | Discard a session |
| `list_sessions` | List every open session with its backend and message count |

### Scratch store (persistent key-value space outside the client's context)
| Tool | Purpose |
|---|---|
| `scratch_set` | Store a value under a key |
| `scratch_get` | Retrieve a value by key |
| `scratch_list` | List all stored keys |
| `scratch_delete` | Delete a key |

### Context budgeting
| Tool | Purpose |
|---|---|
| `estimate_tokens` | Rough heuristic token count for a block of text, to decide whether `compact_context` is worth calling |
| `classify_task_risk` | Fast rule-based check (no model call) flagging whether a task description looks unsafe to delegate (destructive git/DB operations, secrets, architecture decisions) - not authoritative, just a fast first pass |

## Development

```
go build ./...
go vet ./...
go test ./... -race -covermode=atomic
```

`-race` requires cgo (`CGO_ENABLED=1`); on a machine without a C toolchain,
drop `-race` for local runs - CI still runs it on all three OSes.

## Status

v0.1 - core delegation, task orchestration, sessions, and context tools are
in place. See the project's plan file (kept with whoever designed this repo)
for the full architecture rationale.

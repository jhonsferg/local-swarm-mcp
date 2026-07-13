# local-swarm-mcp

An MCP server that delegates mechanical, low-judgment tasks to local or
remote OpenAI-compatible inference backends (llama.cpp's `llama-server`,
Ollama, vLLM, or any hosted provider exposing the same API shape), and
provides context-management tools so an MCP client can manage its own
context budget instead of just accumulating it.

## Why

Judgment-heavy work (architecture decisions, deciding whether a finding is a
false positive, writing meaningful tests) needs a strong model. Mechanical
work (boilerplate generation, log summarization, formatting, repetitive
transforms) doesn't. This server lets an MCP client offload the latter to
whatever hardware you already have running a local model, keeping its own
token budget for the former.

## Tools

| Tool | Purpose |
|---|---|
| `list_backends` | List configured backends + names/models |
| `health_check` | Probe reachability of one or all backends |
| `delegate_task` | Send a task to a backend, get the completion back |
| `compact_context` | Summarize a block of text down to a target size via a backend |
| `scratch_set` / `scratch_get` / `scratch_list` / `scratch_delete` | Persistent local key-value scratch space |
| `estimate_tokens` | Rough heuristic token count for a block of text |
| `classify_task_risk` | Fast rule-based check flagging tasks that look unsafe to delegate |

## Setup

1. Run an OpenAI-compatible inference server somewhere reachable, e.g.:
   ```
   llama-server -m /path/to/model.gguf --host 0.0.0.0 --port 8080
   ```
   or `ollama serve` (already OpenAI-compatible at `/v1`).
2. Copy `config.example.yaml` to `<user config dir>/local-swarm-mcp/config.yaml`
   (Windows: `%APPDATA%\local-swarm-mcp\config.yaml`; Linux/macOS:
   `~/.config/local-swarm-mcp/config.yaml`) and point it at your backend(s).
3. Build and run:
   ```
   go build -o local-swarm-mcp ./cmd/local-swarm-mcp
   ./local-swarm-mcp
   ```
4. Register it as an MCP server with your client (e.g. add an entry to
   Claude Code's `.mcp.json` pointing `command` at the built binary).

## Status

v0.1 - core delegation + context tools. See the project's plan file for the
full architecture rationale and roadmap.

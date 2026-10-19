# Choosing a local model

## Plain delegation (`delegate_task`, `spawn_task`, sessions)

Any instruction-tuned chat model works. Rough guide:

- **1-3B** (e.g. `qwen2.5-coder:1.5b`): short, tightly specified
  transforms - rename, reformat, extract fields, summarize a log. Give a
  strict output format and use `temperature: 0`.
- **7-14B**: boilerplate code from a clear spec, first-draft docs,
  multi-file summaries.
- Bigger models on remote hosts: longer generations, but check
  `base_url` before sending anything sensitive.

The context window is set on the backend (e.g. Ollama's `num_ctx`), not
per call: local-swarm-mcp only sends the OpenAI-standard `max_tokens`,
`temperature`, and `top_p`. If replies look cut off or ignore the start
of a long prompt, the input probably exceeded the model's context - send
less, or `compact_context` it first.

## Tool-using agents (`spawn_agent_task`)

The model must produce real structured `tool_calls`. Verified on this
project:

| Model | Real `tool_calls`? |
|---|---|
| `llama3.1:8b` (Ollama) | yes |
| `qwen2.5-coder:7b` (Ollama) | no - prints tool-call-shaped JSON as text |
| `qwen2.5-coder:1.5b` (Ollama) | no - same failure |

A model that can't call tools "answers" with text that looks like a tool
call, and the task completes with that text as its result and an empty
transcript. Before relying on an unfamiliar model, spawn a trivial agent
task and check its `transcript` is non-empty.

# Troubleshooting

| Symptom | Likely cause | What to do |
|---|---|---|
| `no backends configured` | No static backend and no discovered model | Ask the user to register a host (`register_backend_host`, `-register-host`, or the dashboard) or start the server with `-backend-url`; meanwhile do the work yourself |
| `unknown backend "<host>/<model>"` | Host went offline (its backends drop on the next poll) or the model was removed | `list_backend_hosts` → check `up` / `last_error`; pick another backend |
| `health_check` unhealthy, `connection refused` | The inference server isn't running | Fall back to native work; mention it to the user once |
| `request backend …: context deadline exceeded` | Generation took longer than `-backend-timeout` (default 2m) | Ask for less output (`max_tokens`), split the task, or use a faster model |
| `wait for a free slot on backend …` | The host already has its maximum requests in flight and the wait was cancelled | Wait longer, or spawn fewer tasks at once |
| `wait_task` error with `snapshot=` | Timeout reached; the task is still running | `wait_task` again or `cancel_task` |
| `unknown task` for an ID you had | Finished more than an hour ago, or beyond the 500 kept | Re-run if still needed |
| `scratch store is held by another local-swarm-mcp process` | Another session or the daemon owns the scratch file | Retry later; nothing is lost |
| Agent task result looks like a JSON tool call | The model doesn't support tool calling | See models.md; use `llama3.1:8b` or similar |
| Agent transcript shows `tool "…" is not available to this agent` | The model called a tool outside `mcp_servers` (or made one up) | Expected - the call was refused; tighten the prompt or the filter |
| `register_downstream_mcp_server` isn't listed | The server runs without `-allow-command-registration` | Intended; only the user can enable it |

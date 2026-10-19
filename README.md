# 🐝 local-swarm-mcp

<p align="center">
  <a href="https://github.com/jhonsferg/local-swarm-mcp/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/jhonsferg/local-swarm-mcp/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/jhonsferg/local-swarm-mcp/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/go-1.27%2B-00ADD8?logo=go&logoColor=white">
  <img alt="MCP" src="https://img.shields.io/badge/MCP-compatible-6f42c1">
  <img alt="Status" src="https://img.shields.io/badge/status-v0.5-orange">
</p>

<p align="center">
  <img alt="Ollama" src="https://img.shields.io/badge/Ollama-supported-000000?logo=ollama&logoColor=white">
  <img alt="llama.cpp" src="https://img.shields.io/badge/llama.cpp-supported-333333">
  <img alt="vLLM" src="https://img.shields.io/badge/vLLM-supported-4B8BBE">
  <img alt="OpenAI API" src="https://img.shields.io/badge/API-OpenAI--compatible-412991?logo=openai&logoColor=white">
</p>

<p align="center">
  <img alt="Vue" src="https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white">
  <img alt="Tailwind CSS" src="https://img.shields.io/badge/Tailwind-CSS-06B6D4?logo=tailwindcss&logoColor=white">
  <img alt="Vite" src="https://img.shields.io/badge/Vite-built-646CFF?logo=vite&logoColor=white">
</p>

An MCP server that delegates mechanical, low-judgment tasks to local or
remote OpenAI-compatible inference backends (llama.cpp's `llama-server`,
Ollama, vLLM, or any hosted provider exposing the same API shape), and gives
an MCP client full control over that delegated work: fire-and-forget
background tasks, multi-turn sessions, a persistent scratch store, and
context-budgeting helpers - the same primitives an agent orchestration
system offers for its own subagents, backed by a model running on your own
hardware. 🖥️⚡

## 🧠 Why

Judgment-heavy work (architecture decisions, deciding whether a finding is a
false positive, writing meaningful tests) needs a strong model. Mechanical
work (boilerplate generation, log summarization, formatting, repetitive
transforms) doesn't. This server lets an MCP client offload the latter to
whatever hardware you already have running a local model, keeping its own
token budget for the former - and treat that offloaded work like its own
background agents rather than a single blocking request/response call. 💸

## 🗺️ Architecture

```mermaid
flowchart LR
    subgraph Client["🤖 MCP Client"]
        A["AI Agent<br/>(e.g. Claude Code)"]
    end

    subgraph Server["🐝 local-swarm-mcp"]
        direction TB
        B["MCP Tools<br/>(stdio / HTTP)"]
        C["Backend Registry"]
        D["Task Registry<br/>(async + agent loop)"]
        E["Session Registry<br/>(multi-turn)"]
        F["Scratch Store<br/>(bbolt)"]
        M["Downstream MCP<br/>client manager"]
        B --> C
        B --> D
        B --> E
        B --> F
        D --> M
    end

    subgraph Backends["⚙️ Inference Backends"]
        G["llama.cpp<br/>llama-server"]
        H["Ollama"]
        I["vLLM /<br/>remote provider"]
    end

    subgraph Downstream["🧩 Downstream MCP servers"]
        J["codebase-memory-mcp"]
        K["...other configured servers"]
    end

    A <-->|"MCP protocol"| B
    C -->|"OpenAI-compatible API"| G
    C -->|"OpenAI-compatible API"| H
    C -->|"OpenAI-compatible API"| I
    M <-->|"stdio MCP client"| J
    M <-->|"stdio MCP client"| K
```

Every backend just speaks the OpenAI `/v1/chat/completions` + `/v1/models`
shape, so the server never hardcodes vLLM/llama.cpp/Ollama-specific logic.
Adding a new provider is a config-file edit, not a code change. The downstream
MCP client manager lets a `spawn_agent_task` agent call other MCP servers'
tools autonomously, in a loop, the same way a host application calls its
own tools - see [Tool-using agents](#-tool-using-agents-autonomous-like-a-host-applications-own-agents).

## 🤝 Expected client behavior: check availability, then prefer this over native agents

This server is meant to sit alongside, not replace, an MCP client's own
native agent/subagent capability.

```mermaid
sequenceDiagram
    participant Agent as 🤖 AI Agent
    participant MCP as 🐝 local-swarm-mcp
    participant LLM as 🧩 Local Model

    Agent->>MCP: health_check()
    alt backend healthy
        Agent->>MCP: classify_task_risk(description)
        MCP-->>Agent: risk = low
        Agent->>MCP: spawn_task(prompt)
        MCP-->>Agent: task_id
        MCP->>LLM: POST /v1/chat/completions
        LLM-->>MCP: completion
        Agent->>MCP: wait_task(task_id)
        MCP-->>Agent: result ✅
    else backend unreachable
        Agent->>Agent: fall back to native agent 🔁
    end
```

1. **At the start of a task that could be delegated**, call `list_backends`
   and/or `health_check` to confirm at least one backend is actually
   reachable right now - a configured backend whose `llama-server`/Ollama
   process isn't running, or a remote HTTP transport instance that's
   offline, should not silently block work.
2. **If a backend is healthy**, prefer `spawn_task`/`create_session`
   over the client's own native subagent mechanism for mechanical,
   low-judgment work (use `classify_task_risk` first if unsure whether a
   task qualifies) - this is what actually saves tokens/cost, since the
   work runs on hardware you already own instead of the client's own paid
   model.
3. **If no backend is healthy**, fall back to the client's native agents
   for that work rather than failing the task - this server is a
   cost-saving optimization, not a hard dependency.

Nothing in the server enforces this - it's a client-side policy this
project is designed to support. See a specific client's own configuration
(rules file, memory, system prompt, etc.) for how it's told to apply it.

### 🧩 Agent skill (Claude Code, Codex, Cursor, Copilot, opencode, Gemini CLI)

[`skills/local-swarm/`](skills/local-swarm/) packages this policy as an
[Agent Skill](https://agentskills.io) - the `SKILL.md` format these
coding agents share: when to delegate, which tool fits which job, how to
prompt a small model, how to verify its output, and what never to send
it. Each agent looks in its own directories, but two locations cover all
of them:

| Location | Read by |
|---|---|
| `~/.agents/skills/` (all your projects) or `.agents/skills/` (one project) | Codex, Cursor, GitHub Copilot (agent mode, CLI, cloud agent), opencode, Gemini CLI |
| `~/.claude/skills/` or `.claude/skills/` | Claude Code (also read by Cursor, Copilot, and opencode) |

```sh
# every project, every agent
mkdir -p ~/.agents/skills ~/.claude/skills
cp -r skills/local-swarm ~/.agents/skills/
cp -r skills/local-swarm ~/.claude/skills/
```

The skill assumes the MCP server is registered under the name
`local-swarm-mcp`, as in the examples below.

## ⚙️ Prerequisites

**1. An OpenAI-compatible inference backend, running somewhere reachable.**
local-swarm-mcp does not embed or bundle an inference engine itself - it's a
thin client in front of one. Pick whichever fits your hardware; both were
verified end-to-end against this server during development.

### 🦙 Ollama (easiest - recommended if you just want something working)

Ollama bundles model management and an OpenAI-compatible API, and
auto-detects your GPU backend (CUDA/ROCm/Metal/CPU) with no manual backend
selection.

**Windows:**
```powershell
winget install --id Ollama.Ollama -e
```
**macOS:** download from [ollama.com/download](https://ollama.com/download),
or `brew install ollama`.
**Linux:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

Then pull a small model and start serving (Ollama also auto-starts as a
background service on Windows/macOS after install - `ollama serve` is only
needed if it isn't already running):
```
ollama pull qwen2.5-coder:1.5b   # ~1GB, comfortable on 4-6GB VRAM laptops
ollama serve                      # if not already running as a service
```
Verify it's up:
```
curl http://localhost:11434/v1/models
```
Ollama's OpenAI-compatible endpoint is `http://localhost:11434/v1`.

### 🔧 llama.cpp (better for mixed NVIDIA/AMD hardware)

More manual, but its Vulkan backend runs on both NVIDIA and AMD GPUs without
depending on ROCm's maturity - useful if you have, say, an NVIDIA laptop and
an AMD desktop and want one build that works on both.

```
# build from source (https://github.com/ggml-org/llama.cpp), or grab a
# release binary for your platform, then:
llama-server -m /path/to/model.gguf --host 0.0.0.0 --port 8080
```
Its OpenAI-compatible endpoint is `http://<host>:8080/v1`.

### 🌐 Anything else

**vLLM**, or any hosted provider with an OpenAI-compatible
`/v1/chat/completions` endpoint, also work - just point a backend entry
at it.

> **Automatic discovery is Ollama-only.** `register_backend_host` /
> `-register-host` poll Ollama's native `/api/tags` to find models.
> llama.cpp, vLLM, and hosted providers work as *static* backends -
> `-backend-url`/`-backend-model`, or a `backends:` entry in a config
> file - not as discovered hosts.

**2. local-swarm-mcp itself** - either grab a release binary or build from
source. See [Installing](#-installing) below.

## 📦 Installing

### Linux / macOS

```sh
curl -fsSL https://raw.githubusercontent.com/jhonsferg/local-swarm-mcp/main/install/install.sh | sh
```

### Windows

```powershell
irm https://raw.githubusercontent.com/jhonsferg/local-swarm-mcp/main/install/install.ps1 | iex
```

Both scripts detect your OS/arch, download the matching release archive
and its `checksums.txt`, verify the SHA-256 checksum, and install
`local-swarm-mcp` (`.exe` on Windows) to `~/.local/bin` (Linux/macOS) or
`%USERPROFILE%\.local\bin` (Windows). Set `LSM_INSTALL_DIR`/`LSM_VERSION`
(`$env:LSM_INSTALL_DIR`/`$env:LSM_VERSION` on Windows) before piping to
override the install location or pin a specific version instead of
`latest`.

Releases are built on GitHub's runners by [goreleaser](https://goreleaser.com) for
`{linux,darwin,windows} x {amd64,arm64}` and published automatically on
every merge to `main` whose commits since the last tag warrant a version
bump (Conventional Commits: `feat:` -> minor, `fix:`/`perf:`/`refactor:`
-> patch, `feat!:`/`BREAKING CHANGE` -> major). See [releases](https://github.com/jhonsferg/local-swarm-mcp/releases).

Each release also carries a keyless [Sigstore](https://www.sigstore.dev/)
signature over `checksums.txt`, an SPDX SBOM per archive, and a GitHub
build-provenance attestation per artifact; the release notes show how to
verify them (`cosign verify-blob …` or `gh attestation verify …`). Every
pull request builds the same archives as a snapshot, downloadable from
its CI run.

### 🔐 Security automation

| Workflow | What it does |
|---|---|
| `ci.yml` | Build, vet, race tests on Linux/macOS/Windows; golangci-lint; `govulncheck`; `npm audit`; snapshot release build |
| `codeql.yml` | CodeQL (`security-extended`) for Go, TypeScript, and the workflows themselves; weekly too |
| `trivy.yml` | Dependency CVEs (Go + npm), misconfigurations, and secrets; daily; fails on fixable HIGH/CRITICAL |
| `secrets-scan.yml` | TruffleHog over the git history |
| `dependency-review.yml` | On PRs: blocks new HIGH/CRITICAL advisories and non-allowlisted licenses |
| `.github/dependabot.yml` | Weekly Go and npm updates, monthly Actions updates, with a release-age cooldown |

All third-party actions are pinned to full commit SHAs.

### From source

```sh
go build -o local-swarm-mcp ./cmd/local-swarm-mcp
```
Requires Go 1.27 or newer.

## 🚀 Running the server

There's no required setup and no config file to maintain: just run the
binary. It starts fine with zero backends configured either way - add one
progressively afterward (see below) instead of needing everything up front.

```sh
# The common case: an MCP client (e.g. Claude Code) spawns this itself
# over stdio, per the "Registering with an MCP client" config below - you
# don't normally run this line by hand.
local-swarm-mcp

# Run it as a persistent daemon instead (a standalone process you start
# yourself, with a web dashboard) - useful when your inference hardware
# lives on a different machine than your MCP client, or you just want the
# dashboard. It listens on 127.0.0.1:8090 only; -insecure-no-auth is
# refused on any non-loopback address:
local-swarm-mcp -transport http -insecure-no-auth -ui
```

Several processes can share the same stores safely: a second stdio
session, or a stdio session started while a daemon runs, starts normally.
Only one process can hold the scratch store file at a time, so in the
others `scratch_*` calls return a clear "held by another local-swarm-mcp
process" error - retried on every call, so they start working as soon as
the holder exits - instead of the whole server hanging at startup. Pass
`-store-path` to give a session its own scratch file.

Either way, backends aren't required at startup - see
[Configuring backends](#-configuring-backends) next for how to add one.

## 🛠️ Configuring backends

Register a host - a machine running Ollama (or anything else speaking its
API) - and its models are polled in the background and become usable
automatically, no config file edit or restart needed. This works the same
way regardless of how the server is running:

```sh
# If your MCP client already has local-swarm-mcp running over stdio (the
# common case), just call the register_backend_host tool directly through
# that same connection - no separate command needed.

# Otherwise (a standalone -transport http daemon, or a separate terminal),
# register from the CLI:
local-swarm-mcp -register-host -name local-llama -host-base-url http://localhost:11434

# ...or, if a daemon is running with -ui, from its dashboard's
# "Register host" form instead.
```

`-register-host` either talks to a daemon already running at `-http-addr`
(if one is) or, if none is running yet, becomes the daemon itself - see
[Host discovery](#-host-discovery---add-inference-hardware-without-editing-config-or-restarting)
below for the full mechanics, including what happens if a stdio session
and a separate daemon are both active at once. A single ad-hoc backend
from flags alone (no registration, no persistence) still works too, e.g.
for a quick one-off run:

```
local-swarm-mcp -backend-name local-llama -backend-url http://localhost:8080/v1 -backend-model qwen2.5-coder
```

### Flags that matter most

| Flag | Default | Purpose |
|---|---|---|
| `-transport` | `stdio` | `"stdio"` (spawned as a local subprocess) or `"http"` (the persistent daemon) |
| `-ui` | `false` | Serve the embedded dashboard at `/` - only meaningful for `-transport=http` |
| `-http-addr` | `127.0.0.1:8090` | Listen address when `-transport=http`; a non-loopback address such as `0.0.0.0:8090` requires `-api-key` |
| `-api-key` | *(none)* | Bearer token HTTP clients must present when `-transport=http`; required unless `-insecure-no-auth` is set |
| `-insecure-no-auth` | `false` | Allow `-transport=http` with no `-api-key` - refused unless `-http-addr` is loopback |
| `-allow-command-registration` | `false` | Allow registering downstream MCP servers at runtime (MCP tool, admin API, dashboard) - each registration spawns an arbitrary local command |
| `-register-host` | `false` | Register an inference host for background discovery - see `-name`/`-host-base-url`/`-host-api-key` |
| `-backend-name`/`-backend-url`/`-backend-model`/`-backend-key` | *(none)* | A single ad-hoc backend, for a quick run without the daemon |
| `-backend-timeout` | `2m0s` | Maximum time for a single completion request |
| `-max-concurrent-per-backend` | `4` | Maximum in-flight completions per backend host (`0` = unlimited); extra requests wait their turn |
| `-store-path` | `<user cache dir>/local-swarm-mcp/scratch.db` | Override the scratch-store file location |
| `-lock-path` | `<user cache dir>/local-swarm-mcp/daemon.lock` | Override the daemon lock file (e.g. to run two isolated daemons) |

Run `local-swarm-mcp -h` for the full, grouped flag reference (host
discovery, downstream MCP servers, storage locations, and more).

<details>
<summary>Legacy: YAML/JSON config file</summary>

A `-config path/to/config.yaml` (or `.json`) file with `backends:` and
`mcp_servers:` lists still works and is still auto-detected at
`<user config dir>/local-swarm-mcp/config.yaml` if present - this predates
host discovery and dynamic downstream MCP server registration, and is no
longer the recommended way to configure anything covered by those
features. It remains useful for exactly one thing: a fixed, version-
controllable starting point loaded once at daemon startup (or for a
`-transport stdio` run where a persistent daemon isn't wanted at all).

```yaml
backends:
  - name: local-llama
    base_url: http://localhost:8080/v1
    model: qwen2.5-coder
mcp_servers:
  - name: codebase-memory-mcp
    command: /path/to/codebase-memory-mcp
store_path: C:\Users\you\.cache\local-swarm-mcp\scratch.db
```

⚠️ **This file auto-loads even if you never pass `-config`** - the flag's
*default value* is that same `<user config dir>` path, so if it exists
from an earlier setup, its `backends:` list gets loaded every time and
sits alongside anything registered dynamically, permanently, since a
static entry is never touched by the discovery poller (it isn't the
poller's to manage). If you've moved to host discovery and still see an
old model that no longer exists on some host, this file - not a bug in
discovery - is almost certainly why: either delete it, or empty out its
`backends:` list, and restart the daemon.
</details>

## 🔌 Registering with an MCP client

local-swarm-mcp is a standard MCP server (stdio, or Streamable HTTP at
`/mcp`) and works with any MCP client. CI checks it negotiates every MCP
protocol version from `2024-11-05` to `2025-11-25` and that all its tool
schemas pass the strictest client rules (OpenAI-style function names,
arrays with `items`, 64-character names after client prefixes) - see
`cmd/local-swarm-mcp/compat_test.go`.

### 💻 Local (stdio) - the common case

Point your client at the built binary. Most clients use this shape
(Claude Code's `.mcp.json`, Cursor's `~/.cursor/mcp.json` or
`.cursor/mcp.json`, Claude Desktop, Windsurf, Gemini CLI's
`settings.json`):

```json
{
  "mcpServers": {
    "local-swarm-mcp": {
      "command": "/path/to/local-swarm-mcp"
    }
  }
}
```

The others:

| Client | Where | Entry |
|---|---|---|
| Claude Code (CLI) | - | `claude mcp add local-swarm-mcp -- /path/to/local-swarm-mcp` |
| Codex | `~/.codex/config.toml` (or `codex mcp add local-swarm-mcp -- /path/to/local-swarm-mcp`) | `[mcp_servers.local-swarm-mcp]`<br>`command = "/path/to/local-swarm-mcp"` |
| VS Code / GitHub Copilot (agent mode) | `.vscode/mcp.json` | `{"servers": {"local-swarm-mcp": {"type": "stdio", "command": "/path/to/local-swarm-mcp"}}}` |
| opencode | `opencode.json` | `{"mcp": {"local-swarm-mcp": {"type": "local", "command": ["/path/to/local-swarm-mcp"]}}}` |

Flags go in `args` (`"args": ["-backend-url", "http://localhost:11434/v1", "-backend-model", "qwen2.5-coder:7b"]`)
- for opencode, append them to its `command` array; for Codex, add
`args = [...]`. These files hold per-machine paths (and, for HTTP,
tokens): keep them out of version control.

Keep this path **unversioned** (`local-swarm-mcp`, not
`local-swarm-mcp-v2`): rebuilding to the same fixed path means your
client's config never needs to change and you never need to restart it
just to pick up a new build. Windows won't let you overwrite a `.exe`
its own running process has open, but it *will* let you rename one out
of the way first - so on Windows, rebuild like this instead of building
straight over it:

```powershell
Rename-Item local-swarm-mcp.exe local-swarm-mcp.exe.old -ErrorAction SilentlyContinue
go build -o local-swarm-mcp.exe ./cmd/local-swarm-mcp
```

The already-running process keeps working fine off the renamed file
(Windows processes hold an open handle, not a path) until it's next
restarted, at which point it picks up the new build from the same
stable path - no config edit required.

### 🌍 Remote (HTTP) - running on a separate GPU machine

If your inference hardware lives on a different machine than your MCP
client (e.g. a DGX Spark, or any other PC with a GPU on your network), run
local-swarm-mcp *there* instead, then register that machine's own local
inference server as a host once it's up (from another machine, or from
the dashboard):

```
local-swarm-mcp -transport http -http-addr 0.0.0.0:8090 -api-key <a-strong-random-token> -ui
```

Then point your MCP client at `http://gpu-host:8090/mcp` with an
`Authorization: Bearer <token>` header. Prefer reading the token from an
environment variable over pasting it into a config file:

| Client | Entry |
|---|---|
| Claude Code | `claude mcp add --transport http local-swarm-mcp http://gpu-host:8090/mcp --header "Authorization: Bearer $LOCAL_SWARM_API_KEY"` |
| Cursor (`mcp.json`) | `{"mcpServers": {"local-swarm-mcp": {"url": "http://gpu-host:8090/mcp", "headers": {"Authorization": "Bearer ${env:LOCAL_SWARM_API_KEY}"}}}}` |
| Codex (`config.toml`) | `[mcp_servers.local-swarm-mcp]`<br>`url = "http://gpu-host:8090/mcp"`<br>`bearer_token_env_var = "LOCAL_SWARM_API_KEY"` |
| VS Code / Copilot (`.vscode/mcp.json`) | `{"servers": {"local-swarm-mcp": {"type": "http", "url": "http://gpu-host:8090/mcp", "headers": {"Authorization": "Bearer ${env:LOCAL_SWARM_API_KEY}"}}}}` |
| opencode (`opencode.json`) | `{"mcp": {"local-swarm-mcp": {"type": "remote", "url": "http://gpu-host:8090/mcp", "headers": {"Authorization": "Bearer {env:LOCAL_SWARM_API_KEY}"}}}}` |
| Gemini CLI (`settings.json`) | `{"mcpServers": {"local-swarm-mcp": {"httpUrl": "http://gpu-host:8090/mcp", "headers": {"Authorization": "Bearer $LOCAL_SWARM_API_KEY"}}}}` |

A request without the token gets `401` with a `WWW-Authenticate: Bearer`
challenge. Clients that then offer an OAuth sign-in are probing for an
authorization server this daemon doesn't have - configure the header
instead.

🔒 Listening beyond loopback requires `-api-key` - anyone who can reach
the port could otherwise spawn tasks and read/write the scratch store, so
treat it like any other network-exposed service. `-insecure-no-auth` is
only accepted on a loopback `-http-addr`. The daemon also refuses
requests a web page could forge through your browser: `POST`s that
aren't `application/json`, requests whose `Origin` doesn't match the
daemon, and (on loopback) requests naming a foreign `Host`, which is how
DNS-rebinding attacks reach local servers. API keys - the daemon's own
and any host's - are never returned by any endpoint or tool; responses
only say `"has_api_key": true`.

### 🛰️ Host discovery - add inference hardware without editing config or restarting

Once running with `-transport http`, local-swarm-mcp becomes a small
service-discovery daemon for your inference hosts: register a host once
(a desktop GPU, a DGX Spark, an AMD AI box - anything speaking Ollama's
API) and it's polled in the background forever after. New models pulled
on that host show up automatically as `<host>/<model>` backends - no
YAML/JSON edit, no client restart.

```
local-swarm-mcp -register-host -name rx9070 -host-base-url http://192.168.18.29:11434
```

- If a daemon is already running at the default address, this is a thin
  HTTP client: it forwards the registration and exits.
- If no daemon is running yet, this invocation *becomes* the daemon (same
  process, same leader-election mechanism `-transport http` normally goes
  through) and keeps the registration.

Exactly one process ever wins: every invocation probes `-http-addr` first,
and if nothing answers there, falls back to a PID lockfile
(`<user cache dir>/local-swarm-mcp/daemon.lock`) with a health check as a
second, independent confirmation - a stale lock from a crashed daemon is
reclaimed automatically (its PID is checked for liveness), but a live PID
that isn't actually answering as a real local-swarm-mcp daemon is
reported as a genuine conflict rather than silently overridden.

**A plain `-transport stdio` session gets the same tools too** - it opens
the same persisted host/downstream-server store and runs its own poller,
so `register_backend_host` and friends work directly through the MCP
connection itself, without needing a daemon or the CLI at all. This is
opportunistic rather than exclusive: unlike `-transport http`, a stdio
session doesn't go through leader election first, so if a `-transport
http` daemon is *already* holding that store file when the stdio session
starts, the stdio session just quietly does without those particular
tools for that run (every other tool still works normally, and the
scratch store degrades as described in [Running the server](#-running-the-server))
instead of erroring out or hanging. If you need both at once - a
dashboard *and* an MCP client registering hosts live - start the daemon
first, then your MCP client, and manage hosts from the dashboard or
`-register-host`; the reverse order also works as long as the stdio
session isn't holding the host store at the moment the daemon starts
(the daemon retries for up to 5s before giving up with a clear error).

`-register-host` must present the same `-api-key` as the running daemon;
without it, it stops with an error saying so.

The same operations are also available as MCP tools
(`register_backend_host`, `unregister_backend_host`,
`list_backend_hosts`) so an agent - or you, through your MCP client - can
manage hosts directly over the existing connection. `list_backend_hosts`
reports whether each host is currently reachable (`up`), so you (or an
agent orchestrating work) can tell a host that's merely registered but
powered off from one that's actually available before dispatching a task
to it.

### 🔌 Downstream MCP servers, dynamically

The `mcp_servers:` config-file list works the same way it always has, but
it's legacy now: `register_downstream_mcp_server` (and its
`unregister_downstream_mcp_server` / `list_downstream_mcp_servers`
counterparts) registers a server like `codebase-memory-mcp` at runtime -
spawned and connected immediately, persisted so it reconnects
automatically across daemon restarts, no config file or restart needed.
The same three operations are also exposed over the admin HTTP surface
(`POST /admin/register-mcp-server`, `POST /admin/unregister-mcp-server`,
`GET /admin/mcp-servers`) and from the embedded dashboard below.

🔒 **Registering is off unless you start with `-allow-command-registration`.**
A registration is "run this command", so without the flag the MCP tool
isn't offered at all and the admin endpoint answers 403 - a misled or
prompt-injected client can't turn it into arbitrary command execution.
Listing and unregistering always work, and servers from a config file or
registered earlier still connect. A server whose process dies is respawned
on its next tool call; names may not contain `__` (it separates server and
tool in `<server>__<tool>`).

## 📊 Dashboard

```
local-swarm-mcp -transport http -insecure-no-auth -ui
```

`-ui` serves an embedded dashboard at `/` (a small Vue app - see
[`ui/`](ui/)) showing configured backends, registered inference hosts
with live up/down status and discovered models, registered downstream
MCP servers with live connected/disconnected status, and a live-streaming
log tail. Everything on the page is a thin client over the same
`/admin/*` endpoints described above - you can register or edit a host,
register a downstream MCP server (with `-allow-command-registration`), or
watch what the daemon is doing in real time, all without touching a
config file or a terminal. Failed actions are shown in a banner at the
top of the page.

With `-api-key`, the page asks for the key once and the daemon keeps an
`HttpOnly`, `SameSite=Strict` session cookie (derived from the key, never
the key itself) for every later request, including the live log stream.
The page's static files are public; all data stays behind the key.
Host API keys are never sent to the page: when editing a host, leave the
key field blank to keep its current key.

## 🧰 Tools

### 🔗 Backends
| Tool | Purpose |
|---|---|
| `list_backends` | List configured backends (name, base_url, model, whether a key is set) - the first one listed is the default |
| `health_check` | Probe reachability of one backend, or all if omitted |

Every tool with a `backend` parameter falls back to a default when it's
omitted: the first statically configured backend (`-backend-url` or a
config file), or - with none - the first discovered `<host>/<model>`
backend in name order. Completions are capped at
`-max-concurrent-per-backend` in flight per backend host and
`-backend-timeout` each.

### 🛰️ Host discovery
| Tool | Purpose |
|---|---|
| `register_backend_host` | Register a new inference host for background model discovery |
| `unregister_backend_host` | Remove a registered host and stop polling it |
| `list_backend_hosts` | List registered hosts, whether each is currently reachable, and the models discovered on it |

Available on every transport, including the default `-transport stdio` -
each session runs its own poller against the same persisted store, so a
host registered from one session (or from the dashboard) is there the next
time any session opens that store, no restart needed. See
[Host discovery](#-host-discovery---add-inference-hardware-without-editing-config-or-restarting)
below for the one edge case (two sessions racing for the same store file).

### 🔌 Downstream MCP servers
| Tool | Purpose |
|---|---|
| `register_downstream_mcp_server` | Register and connect a downstream MCP server (e.g. codebase-memory-mcp) at runtime - only offered with `-allow-command-registration` |
| `unregister_downstream_mcp_server` | Disconnect and remove a registered downstream MCP server |
| `list_downstream_mcp_servers` | List registered downstream servers and whether each is currently connected |

### ⚡ One-shot delegation
| Tool | Purpose |
|---|---|
| `delegate_task` | Send a task to a backend and block for the completion - the simple synchronous path |
| `compact_context` | Summarize a block of text down to a target size via a backend, so it doesn't sit uncompacted in the client's own context |

`delegate_task`, `spawn_task`, `send_message`, and `spawn_agent_task` all take
the same three sampling parameters: `max_tokens`, `temperature`, `top_p`.
`temperature` is always sent, so `0` really means deterministic decoding;
`top_p` is omitted unless set.
Deliberately only these three - they're the ones the OpenAI
chat-completions spec actually defines, so they're honored consistently
whether the backend is llama.cpp, Ollama, vLLM, or a hosted provider.
Ollama-specific knobs like `num_ctx` (context window size) or `top_k`
aren't part of that spec: verified live, Ollama's OpenAI-compatible
endpoint accepts them in an `options` object without erroring but
silently ignores them, so they're not exposed here - a parameter that
looks like it works but quietly doesn't would be worse than not having
it. Tuning those for real currently means either building a derived
model with `ollama create` and its own baked-in `PARAMETER num_ctx ...`,
or calling the backend's native API directly rather than through
local-swarm-mcp.

### 🎯 Background tasks (fire-and-forget, like spawning a subagent)
| Tool | Purpose |
|---|---|
| `spawn_task` | Start a task in the background, return a task ID immediately |
| `task_status` | Non-blocking snapshot of a task's state (pending/running/completed/failed/cancelled) |
| `wait_task` | Block until a task finishes or a timeout elapses, then return its final snapshot |
| `list_tasks` | List tasks spawned this server run |
| `cancel_task` | Cancel a still-running task |

Tasks live in memory only. Running tasks are never dropped; finished ones
stay queryable for an hour, at most the 500 most recent.

### 🧑‍🔧 Tool-using agents (autonomous, like a host application's own agents)
| Tool | Purpose |
|---|---|
| `spawn_agent_task` | Start a background agent that can call tools from configured downstream MCP servers in a loop before answering, tracked via the same `task_status`/`wait_task`/`list_tasks`/`cancel_task` tools above. Its `mcp_servers` filter is enforced: a call to any tool the agent wasn't offered is refused and recorded as an error. Each tool result fed back to the model is capped at 16 KiB. The result includes a full transcript of every tool call made, for auditing. |
| `list_available_agent_tools` | List every tool discovered from configured downstream MCP servers (namespaced `<server>__<tool>`) that an agent could invoke |

⚠️ **Requires a tool-calling-capable model.** Not every model that claims
"tools" support actually produces a real structured `tool_calls` response.
Verified on this project:

| Model | Real `tool_calls`? |
|---|---|
| `llama3.1:8b` (Ollama) | ✅ Yes |
| `qwen2.5-coder:7b` (Ollama) | ❌ No - prints tool-call-shaped JSON as plain text |
| `qwen2.5-coder:1.5b` (Ollama) | ❌ No - same failure mode |

Test a new model with a trivial tool schema before relying on it for
`spawn_agent_task` - a model that fails silently prints its "call" as text
and the loop just treats it as (probably wrong) final output.

Example config wiring `codebase-memory-mcp` (installed earlier, see its own
docs) as a downstream server:
```yaml
backends:
  - name: agent-model
    base_url: http://localhost:11434/v1
    model: llama3.1:8b

mcp_servers:
  - name: codebase-memory-mcp
    command: /path/to/codebase-memory-mcp
```

🔒 Downstream tool access is opt-in and configured per server; `codebase-memory-mcp`
is a good reference integration because its tools are read-only (indexing
and querying, no writes). Think carefully before pointing an agent task at
a downstream server with write/destructive tools - the model driving it may
be far less reliable than the client that configured it.

### 💬 Sessions (persistent multi-turn conversations, like resuming a named agent)
| Tool | Purpose |
|---|---|
| `create_session` | Open a session against a backend with an optional system prompt |
| `send_message` | Send a message within a session, carrying its full prior history, and get the reply |
| `session_history` | Return a session's full message history |
| `close_session` | Discard a session |
| `list_sessions` | List every open session with its backend and message count |

A session keeps (and resends) only its most recent 40 history messages,
dropped a user/assistant pair at a time; its system prompt always stays.

### 🗄️ Scratch store (persistent key-value space outside the client's context)
| Tool | Purpose |
|---|---|
| `scratch_set` | Store a value under a key |
| `scratch_get` | Retrieve a value by key |
| `scratch_list` | List all stored keys |
| `scratch_delete` | Delete a key |

### 📊 Context budgeting
| Tool | Purpose |
|---|---|
| `estimate_tokens` | Rough heuristic token count for a block of text, to decide whether `compact_context` is worth calling |
| `classify_task_risk` | Fast rule-based check (no model call) flagging whether a task description looks unsafe to delegate (destructive git/DB operations, secrets, architecture decisions) - not authoritative, just a fast first pass |

## 🧪 Development

```
go build ./...
go vet ./...
go test ./... -race -covermode=atomic
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...
```

`-race` requires cgo (`CGO_ENABLED=1`); on a machine without a C toolchain,
drop `-race` for local runs - CI still runs it on all three OSes.

`go test ./...` includes end-to-end tests (`cmd/local-swarm-mcp/integration_test.go`)
that build the real binary and drive it over stdio and HTTP against fake
backends, with every store, lock, and log path in a temp dir - they never
touch your real local-swarm-mcp state.

### Building the dashboard (`ui/`)

The embedded dashboard is a separate Vue 3 + TypeScript + Tailwind
project at [`ui/`](ui/), kept independent from the Go server so each side
has its own fast test loop (`vitest` for UI components, `go test` for the
server) instead of one slow, tangled one.

```
cd ui
npm ci
npm run test     # vitest, component-level
npm run build    # type-checks (vue-tsc) then builds straight into
                  # ../internal/webui/dist/app, which cmd/local-swarm-mcp
                  # embeds via go:embed
```

The build output is **never committed** (`internal/webui/dist/app/` is
git-ignored; only `internal/webui/dist/.gitkeep` is tracked so
`go:embed` compiles). A bare `go build ./...` on a fresh clone without
Node still works - its `-ui` page just explains how to build the real
dashboard, and `local-swarm-mcp -version` reports `dashboard not built`.
Run `npm run build` before `go build` to embed it. CI and the release
workflow always do, and fail if a published binary doesn't report
`dashboard embedded`.

For local frontend development against a running daemon, `npm run dev`
starts Vite's dev server with `/admin` proxied to `http://localhost:8090`
(see `ui/vite.config.ts`) - point it at a different `-http-addr` by
editing that proxy target.

## 📌 Status

v0.5 - core delegation, task orchestration, sessions, context tools,
tool-using agents, a release pipeline, multi-host inference discovery,
dynamic downstream MCP server registration, structured logging, and an
embedded dashboard are all in place. Verified end-to-end over stdio and
HTTP against real Ollama backends across two machines (a 6GB-VRAM laptop
GPU and a 16GB-VRAM desktop GPU on the same LAN):
- v0.1: all 20 tools registered and responded correctly against `qwen2.5-coder:1.5b`,
  including a full spawn/wait task round-trip and a multi-turn session.
- v0.2: a `spawn_agent_task` run against `llama3.1:8b`, with `codebase-memory-mcp`
  wired in as a downstream server, correctly called `index_repository` on
  this repo and reported the real result (288 nodes, 937 edges) rather than
  a hallucinated one, with the tool call recorded in the task's transcript.
- v0.3: an auto-release pipeline (Conventional Commits bump detection,
  goreleaser) and one-line install scripts for Linux/macOS and Windows,
  both verified against real goreleaser-produced archives and checksums.
- v0.4: `-register-host` against a real remote Ollama instance - both the
  client path (an existing daemon forwards the registration) and the
  daemon-election path (no daemon running yet, the invocation becomes
  one) verified live; the background poller correctly discovered a
  freshly-pulled model (`gemma4:12b`) on that host and made it callable
  as `rx9070/gemma4:12b` without any config edit or restart.
- v0.5: `-ui` verified live against a real daemon and a real remote host -
  registering, editing, and removing a host and a downstream MCP server
  from the dashboard all worked end-to-end through the `/admin/*`
  endpoints, live log streaming (SSE) showed each action as it happened,
  and a `goreleaser build --snapshot` produced a release binary that
  served the real (non-placeholder) dashboard, confirming the whole
  `ui/` → `internal/webui/dist` → `go:embed` pipeline works outside CI too.

## 📄 License

MIT - see [LICENSE](LICENSE).

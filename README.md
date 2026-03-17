# Golem

**Golem is a terminal-native coding agent for real repositories.** It launches in your shell, understands repo context before the first prompt, and keeps sessions, memory, and durable mission state on your machine.

It is built for developers who want an agent that feels like part of the terminal workflow instead of a browser-heavy IDE.

<img width="1710" height="1107" alt="Golem terminal UI" src="https://github.com/user-attachments/assets/c94e7707-df8f-4be9-91f2-1d0a4b27a17d" />

## Why Golem

Golem front-loads the things that matter in day-to-day repository work:

- **Terminal-native by default** — launch a TUI, type prompts, use slash commands, and stay in the shell.
- **Repo-aware on startup** — Golem loads git state, saved sessions, project instructions, and local memory before helping.
- **Local-first state** — auth, sessions, memory, missions, and automation config live under `~/.golem/`.
- **Recover prior work** — use `/resume`, `/search <query>`, `/replay`, and `/rewind` to find and restore earlier sessions.
- **Durable mission workflows** — create longer-running work with `/mission ...` in the main TUI and inspect it in Mission Control with `golem dashboard`.
- **Operator-friendly UX** — `/help`, `/doctor`, runtime summaries, model switching, cost tracking, verification state, and explicit mission controls are shipped surfaces.

## What you can do today

With the currently shipped CLI and TUI, you can:

- work interactively in a repository with an agent that can read, search, edit, write, list, and run commands,
- start the app with an initial prompt from the shell,
- check setup and runtime quickly with `golem status`, `golem runtime`, and `/doctor`,
- search earlier saved sessions with `/search <query>` and recover work with `/resume`, `/replay`, and `/rewind`,
- run durable mission workflows from the main TUI with `/mission ...` and inspect them in **Mission Control** with `golem dashboard`,
- configure local automation workflows with `golem automations ...`.

## First successful session

This is the shortest path from clone to a useful first run.

### 1. Install

**Prerequisites**

- Go **1.26+**
- a git repository you want to work in
- one supported model provider configured with credentials

```bash
go install github.com/fugue-labs/golem@latest
# or build from source
go build -o golem .
```

Prefer a prebuilt binary? Download the macOS Apple Silicon (`darwin-arm64`) or Linux x86_64 (`linux-amd64`) archive from this repository's GitHub Releases page.

### 2. Authenticate

Fastest path:

```bash
golem login
```

You can also choose a specific shipped login flow:

```bash
golem login chatgpt
golem login anthropic
golem login openai
golem login xai
```

What those flows do:

- `chatgpt` uses browser-based OAuth and saves credentials in `~/.golem/auth.json`
- `anthropic`, `openai`, and `xai` prompt for an API key and save it in `~/.golem/credentials.json`
- successful login saves your provider preference in `~/.golem/config.json`

If you prefer environment variables, Golem also auto-detects runtime configuration.

| Runtime provider | What to set |
| --- | --- |
| Anthropic | `ANTHROPIC_API_KEY=...` |
| OpenAI | `OPENAI_API_KEY=...` |
| OpenAI-compatible / xAI-style endpoint | `GOLEM_PROVIDER=openai_compatible`, `GOLEM_API_KEY=...`, `GOLEM_BASE_URL=...` |
| Vertex AI | `GOLEM_PROVIDER=vertexai`, `VERTEX_PROJECT=...` |
| Vertex AI Anthropic | `GOLEM_PROVIDER=vertexai_anthropic`, `VERTEX_PROJECT=...` |

Optional overrides include `GOLEM_MODEL`, `VERTEX_REGION`, `GOLEM_TIMEOUT`, and `GOLEM_PERMISSION_MODE`.

### 3. Sanity-check the runtime

```bash
golem status
golem runtime
```

Need machine-readable output?

```bash
golem status --json
golem runtime --json
```

### 4. Launch inside a repository

```bash
cd path/to/repo
golem
```

Or start with an initial prompt:

```bash
golem "summarize this repository and suggest the next three tasks"
```

### 5. Inside the app, hit the success path

Start with these shipped slash commands:

```text
/help
/doctor
/search <query>
/mission new improve onboarding docs
```

What to expect:

- `/help` shows the current command surface and keybindings.
- `/doctor` checks setup and helps explain configuration problems.
- `/search <query>` searches across saved sessions; on a brand-new install it is still a good way to learn the search surface before you have history.
- `/mission new <goal>` creates a durable mission you can inspect with `/mission status`, `/mission tasks`, and `golem dashboard`.

## Core command surfaces

### CLI

| Command | What it does |
| --- | --- |
| `golem` | Launch the main TUI |
| `golem "<prompt>"` | Launch the TUI with an initial prompt |
| `golem login [chatgpt\|anthropic\|openai\|xai]` | Run a shipped login flow |
| `golem logout` | Remove saved login/config files under `~/.golem/` |
| `golem status [--json]` | Show runtime readiness and configuration summary |
| `golem runtime [--json]` | Show the effective runtime profile |
| `golem dashboard` | Open Mission Control |
| `golem automations list` | List configured automations |
| `golem automations start` | Start the automations daemon |
| `golem automations status` | Show automation runtime status |
| `golem automations init` | Print an example automations config |

### In-app slash commands

Common starting points inside the TUI:

| Command | What it does |
| --- | --- |
| `/help` | Show available commands and keys |
| `/doctor` | Diagnose setup issues |
| `/search <query>` | Search across all saved sessions |
| `/resume` | Restore the last saved session |
| `/replay [file\|list]` | Replay a recorded session trace |
| `/rewind [N]` | Rewind to an earlier checkpoint |
| `/model [name]` | Show or switch the active model |
| `/cost` | Show session cost breakdown |
| `/runtime` | Show the effective runtime profile |
| `/diff` | Show git diff stats for uncommitted changes |
| `/undo [path]` | Revert one unstaged tracked-file change |
| `/mission [new\|status\|tasks\|plan\|approve\|start\|pause\|cancel\|retry [task-id]\|list]` | Operate the durable mission workflow |
| `/clear` | Clear the current transcript |
| `/quit` | Exit the app |

For the full shipped command and keybinding reference, see [docs/command-reference.md](docs/command-reference.md).

## Supported workflows

### Interactive repo work

Use Golem as a local coding partner that starts with repo context instead of a blank chat box. It detects git state, loads project instructions, and works directly against your current checkout.

### Session recovery and search

Saved sessions are searchable and replayable. Use `/search <query>` to find prior work, `/resume` to continue the last session, `/replay` to inspect earlier traces, and `/rewind` to return to a checkpoint.

### Durable missions

For longer-running work, create a mission from the main TUI:

```text
/mission new ship the onboarding docs refresh
```

Then continue with:

```text
/mission status
/mission tasks
/mission plan
/mission approve
/mission start
```

Mission Control is available at:

```bash
golem dashboard
```

The dashboard shows mission status plus **Tasks**, **Workers**, **Evidence**, and **Events** panes.

### Local automations

Use `golem automations list`, `golem automations init`, `golem automations status`, and `golem automations start` to inspect and run local automation workflows.

## Local-first state

Golem keeps its operator state under `~/.golem/`, including:

- authentication data,
- saved sessions,
- project memory,
- durable mission state,
- automation configuration.

That local-first model is a big part of the product: you can close the app, come back later, search prior work, and continue from durable state.

## Where to go next

- [Getting started](docs/getting-started.md) — full onboarding, provider setup, and first-run details
- [Command reference](docs/command-reference.md) — authoritative CLI, slash-command, mission, and keybinding reference
- [Configuration](docs/configuration.md) — environment variables, config, permissions, and runtime tuning
- [Features](docs/features.md) — deeper product and UX surface documentation

If you want a quick health check before doing real work, start with `golem status`, launch `golem`, run `/help`, and then use `/doctor`, `/search <query>`, or `/mission new` depending on what you want to do next.

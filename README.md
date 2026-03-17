# Golem

Golem is a terminal-native coding agent for real software projects. It combines a fast chat-first TUI, repo-aware tooling, mission orchestration, persistent session search, and practical guardrails so you can ship changes without leaving your terminal.

This repository contains the `golem` CLI and TUI app.

## Why Golem

- **Terminal-first**: chat, inspect files, run workflows, and manage missions without switching tools.
- **Repo-aware**: understands git state, project instructions, saved sessions, and persistent memory.
- **Operator-friendly**: slash commands, status summaries, cost tracking, verification state, and a dashboard for long-running work.
- **Local-first workflows**: missions, sessions, memory, and automations are stored under `~/.golem/`.
- **Model-flexible**: supports Anthropic, OpenAI, OpenAI-compatible providers such as xAI, and Vertex AI.

## Highlights

- Interactive TUI with transcript, input history, paging, slash-command completion, and cancellation.
- Built-in code tools for reading, searching, editing, writing, listing, and running commands in your repo.
- Mission orchestration with durable task planning and a separate **Mission Control** dashboard.
- Session replay, rewind, resume, and cross-session search.
- Verification, invariant tracking, budgeting, and per-session cost summaries.
- Optional automations for webhook- or schedule-driven workflows.

## Installation

For the full onboarding flow, including provider selection, authentication choices, environment-variable setup, and first-session checks, start with the dedicated guide:

- [Getting started with Golem](docs/getting-started.md)

### Prerequisites

- Go **1.26+**
- One supported model provider configured with credentials

### Build from source

```bash
go build -o golem .
```

You can also install it directly:

```bash
go install github.com/fugue-labs/golem@latest
```

## Quick start

If you already know how you want to authenticate, the short version is:

```bash
golem login
golem
```

Inside the app, start with:

```text
/help
/runtime
/doctor
```

Useful first commands:

- `/help`
- `/search <query>`
- `/doctor`
- `/runtime`
- `/plan`
- `/cost`
- `/mission new <goal>`

## Core commands

### Shell commands

```bash
golem                     # launch the TUI
golem login               # interactive provider login
golem logout              # remove saved local auth/config
golem status              # one-shot status summary
golem runtime             # one-shot runtime profile
golem dashboard           # open Mission Control
golem automations list    # list configured automations
golem automations init    # print an example automations config
golem automations status  # show daemon status
```

For machine-readable status output:

```bash
golem status --json
golem runtime --json
```

### In-app slash commands

| Command | What it does |
| --- | --- |
| `/help` | Show commands and keybindings |
| `/clear` | Clear the current transcript |
| `/plan` | Show tracked plan progress |
| `/invariants` | Show tracked hard/soft constraints |
| `/runtime` | Show effective runtime configuration |
| `/verify` | Show latest verification summary |
| `/compact` | Compress conversation context |
| `/cost` | Show session cost summary |
| `/budget` | Show budget status and fallback info |
| `/resume` | Restore the last saved session |
| `/search <query>` | Search across saved sessions |
| `/model [name]` | Show or switch the active model |
| `/diff` | Show uncommitted git diff |
| `/undo [path]` | Revert unstaged changes |
| `/replay [file\|list]` | Replay a recorded session trace |
| `/rewind [N]` | Rewind to a checkpoint or list checkpoints |
| `/doctor` | Diagnose setup issues |
| `/config` | Show effective configuration |
| `/team` | Show team status |
| `/context` | Show context usage |
| `/skills` | List detected skills |
| `/skill <name>` | Toggle a skill |
| `/spec [file]` | Start or show a spec workflow |
| `/mission [new\|status\|tasks\|plan\|approve\|start\|pause\|cancel\|list]` | Run mission workflows |
| `/quit` or `/exit` | Quit Golem |

### Keybindings

| Key | Action |
| --- | --- |
| `Enter` | Send message |
| `Shift+Enter` | Insert newline |
| `Tab` | Autocomplete slash commands |
| `Esc` | Cancel the active run |
| `Ctrl+L` | Clear the transcript |
| `↑ / ↓` | Recall input history |
| `PgUp / PgDn` | Scroll the transcript |

## Mission Control

Golem includes a durable mission system for longer-running, multi-step work.

From the main TUI you can:

```text
/mission new Build a REST API with authentication
/mission status
/mission tasks
/mission plan
/mission approve
/mission start
/mission pause
/mission cancel
/mission list
```

You can inspect the same mission state in a dedicated dashboard:

```bash
golem dashboard
```

The dashboard shows mission status plus **Tasks**, **Workers**, **Evidence**, and **Events** panes.

## Configuration

Golem supports both saved config and environment variables.

### Common environment variables

| Variable | Purpose |
| --- | --- |
| `GOLEM_PROVIDER` | Force the provider (`anthropic`, `openai`, `openai_compatible`, `vertexai`, `vertexai_anthropic`) |
| `GOLEM_MODEL` | Override the active model |
| `GOLEM_ROUTER_MODEL` | Set a cheaper routing model |
| `GOLEM_BASE_URL` | Custom OpenAI-compatible endpoint |
| `GOLEM_TIMEOUT` | Request timeout, e.g. `30m` |
| `GOLEM_PERMISSION_MODE` | Permission mode, typically `suggest` or `auto` |
| `GOLEM_TEAM_MODE` | Team mode (`auto`, `on`, `off`) |
| `GOLEM_SESSION_BUDGET` | Per-session budget in USD |
| `GOLEM_PROJECT_BUDGET` | Per-project budget in USD |
| `GOLEM_BUDGET_WARN_PCT` | Budget warning threshold |
| `GOLEM_FALLBACK_MODEL` | Model to switch to when budget pressure is high |
| `GOLEM_REASONING_EFFORT` | OpenAI reasoning effort |
| `GOLEM_THINKING_BUDGET` | Thinking budget for Anthropic / Gemini-style models |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENAI_BASE_URL` | OpenAI-compatible base URL |
| `XAI_API_KEY` | xAI API key |
| `XAI_BASE_URL` | xAI base URL |
| `VERTEX_PROJECT` | Google Cloud project for Vertex AI |
| `VERTEX_REGION` | Vertex AI region |

### Project instructions

On startup, Golem loads project instructions from locations such as:

- `GOLEM.md`
- `CLAUDE.md`
- `.golem/instructions.md`
- `~/.golem/instructions.md`

Parent-directory instructions up to the git root are also considered.

## Local data directory

Golem stores local state under `~/.golem/`, including:

- `config.json` — saved provider and budget preferences
- `credentials.json` — API keys
- `auth.json` — ChatGPT OAuth credentials
- `sessions/` — saved sessions for resume/search/replay
- `memory/` — project-scoped persistent memory
- `missions.db` — durable mission store
- `automations.json` — automation config

## Automations

Golem can run automation workflows from a local config file:

```bash
golem automations init
```

That prints an example `~/.golem/automations.json`. After configuring it, you can inspect the setup with:

```bash
golem automations list
golem automations status
```

## Development

Build the project:

```bash
go build ./...
```

Run the test suite:

```bash
go test ./...
```

## Release notes for this repo

- The module depends on **`github.com/fugue-labs/gollem v0.3.0`**.
- The previous local `replace` directive has been removed so release builds resolve Gollem from the published module version.

## Documentation

Additional internal documentation lives in `docs/`, including feature notes and implementation specs.

## Contributing

Issues and pull requests are welcome. If you are working on behavior that affects the terminal UX, mission workflows, or command surfaces, please preserve the existing command contracts and keep `go test ./...` green.

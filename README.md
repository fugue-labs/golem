# Golem

Golem is a terminal-native coding agent for real repositories. It gives you a chat-first TUI, repo-aware context, local session history, and durable mission workflows without sending you to a browser-heavy IDE.

This repository contains the `golem` CLI and TUI app.

## Why Golem

Golem is built for people who want an agent that feels like part of the terminal workflow instead of a separate product.

- **Chat-first, terminal-native** — launch the TUI, type a prompt, use slash commands, and keep working in the same shell-centric loop.
- **Repo-aware by default** — Golem reads git state, project instructions, saved sessions, and runtime configuration before it starts helping.
- **Operator-friendly** — built-in help, diagnostics, runtime/status summaries, cost tracking, verification state, and explicit mission controls are part of the shipped surface.
- **Local-first state** — sessions, credentials, memory, missions, and automation config live under `~/.golem/`.
- **More than one-shot chat** — search older sessions, replay traces, rewind to checkpoints, and manage larger work through durable missions.

## Shipped workflows

With Golem you can:

- work interactively in a repo with an agent that can read, search, edit, write, list, and run commands,
- validate setup quickly with `golem status`, `golem runtime`, and `/doctor`,
- recover earlier work with `/resume`, `/search <query>`, `/replay`, and `/rewind`,
- run durable mission workflows from the main TUI with `/mission ...` and inspect them in **Mission Control** with `golem dashboard`,
- configure local automation workflows with `golem automations ...`.

## Install

### Prerequisites

- Go **1.26+**
- One supported model provider configured with credentials

### Build from source

```bash
go build -o golem .
```

### Install directly

```bash
go install github.com/fugue-labs/golem@latest
```

Prefer a prebuilt binary? Download the macOS Apple Silicon (`darwin-arm64`) or Linux x86_64 (`linux-amd64`) archive from this repository's GitHub Releases page.

## Authentication and provider setup

### Fastest path: `golem login`

```bash
golem login
```

Or choose a specific shipped login flow:

```bash
golem login chatgpt
golem login anthropic
golem login openai
golem login xai
```

What these do:

- `chatgpt` uses browser-based OAuth and stores credentials in `~/.golem/auth.json`
- `anthropic`, `openai`, and `xai` prompt for an API key and store it in `~/.golem/credentials.json`
- successful login saves your provider preference in `~/.golem/config.json`

If you prefer environment variables, Golem also auto-detects credentials at runtime.

### Runtime providers

Shipped runtime paths support:

- **Anthropic**
- **OpenAI**
- **OpenAI-compatible / xAI**
- **Vertex AI**
- **Vertex AI Anthropic**

Important distinction: `golem login` supports `chatgpt`, `anthropic`, `openai`, and `xai`. Vertex providers are configured via environment variables.

### Useful auth/config env vars

| Variable | Purpose |
| --- | --- |
| `GOLEM_PROVIDER` | Explicitly override provider selection |
| `GOLEM_MODEL` | Override the active model |
| `GOLEM_BASE_URL` | Custom OpenAI-compatible endpoint |
| `GOLEM_API_KEY` | API key for OpenAI-compatible providers |
| `GOLEM_TIMEOUT` | Request timeout, for example `30m` |
| `GOLEM_PERMISSION_MODE` | Permission mode such as `suggest` or `auto` |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `XAI_API_KEY` | xAI API key |
| `VERTEX_PROJECT` | Google Cloud project for Vertex AI |
| `VERTEX_REGION` | Vertex AI region |

Use `golem status`, `golem runtime`, and `/config` to confirm the effective configuration.

## First successful session

If you want the shortest path from install to a useful first run, do this in a repo.

### 1. Log in

```bash
golem login
```

### 2. Confirm your runtime is valid before launching

```bash
golem status
golem runtime
```

Use `--json` if you want machine-readable output:

```bash
golem status --json
golem runtime --json
```

### 3. Launch Golem

```bash
golem
```

Or start with an initial prompt:

```bash
golem fix the failing tests
```

### 4. Inside the TUI, do these first

```text
/help
/doctor
/search <query>
/mission new Fix the flaky integration tests
```

What each one gives you:

- `/help` — discover commands and keybindings
- `/doctor` — diagnose auth, repo, instructions, and tool availability issues
- `/search <query>` — search across saved sessions for earlier fixes and context
- `/mission new <goal>` — create a durable mission for larger multi-step work

If `/search <query>` returns nothing useful yet, that is expected on a brand-new setup. It becomes more valuable after Golem has saved sessions.

### 5. Ask for real work

Examples:

```text
Summarize this repository and identify the riskiest area.
Add a failing test for the bug in the session loader.
Refactor the login flow and keep go test ./... green.
```

## Core CLI commands

| Command | What it does |
| --- | --- |
| `golem` | Launch the main TUI |
| `golem <prompt>` | Launch the TUI with an initial prompt |
| `golem login [provider]` | Run the interactive login flow |
| `golem logout` | Remove saved local auth/config files |
| `golem status [--json]` | Show a one-shot status summary |
| `golem runtime [--json]` | Show the effective runtime profile |
| `golem dashboard [mission-id]` | Open Mission Control |
| `golem automations [list]` | List configured automations |
| `golem automations start` | Start the automation daemon |
| `golem automations status` | Show automation daemon status |
| `golem automations init` | Print an example automations config |

`golem logout` removes saved files under `~/.golem/`, but it does **not** clear environment variables.

## Useful slash commands in the TUI

| Command | What it does |
| --- | --- |
| `/help` | Show commands and keybindings |
| `/clear` | Clear the current transcript |
| `/plan` | Show tracked plan progress |
| `/runtime` | Show the effective runtime profile |
| `/verify` | Show the latest verification summary |
| `/cost` | Show session cost summary |
| `/budget` | Show budget status |
| `/resume` | Restore the last saved session |
| `/search <query>` | Search across saved sessions |
| `/model [name]` | Show or switch the active model |
| `/diff` | Show uncommitted git diff |
| `/undo [path]` | Revert an unstaged change |
| `/replay [file\|list]` | Replay a recorded session trace |
| `/rewind [N]` | Rewind to a checkpoint or list checkpoints |
| `/doctor` | Diagnose setup issues |
| `/config` | Show effective configuration |
| `/context` | Show context usage |
| `/skills` | List detected skills |
| `/skill <name>` | Toggle a skill |
| `/spec [file]` | Start or show a spec workflow |
| `/mission [new\|status\|tasks\|plan\|approve\|start\|pause\|cancel\|list]` | Run mission workflows |
| `/quit` or `/exit` | Quit Golem |

## Durable mission workflow

For bigger tasks, start in the main TUI:

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

Inspect durable mission state in a separate dashboard:

```bash
golem dashboard
```

Mission Control shows mission status plus **Tasks**, **Workers**, **Evidence**, and **Events** panes.

## Keybindings

| Key | Action |
| --- | --- |
| `Enter` | Send message |
| `Shift+Enter` | Insert newline |
| `Tab` | Autocomplete slash commands |
| `Esc` | Cancel the active run |
| `Ctrl+L` | Clear the transcript |
| `↑ / ↓` | Recall input history |
| `PgUp / PgDn` | Scroll the transcript |

## Local data and project instructions

Golem stores local state under `~/.golem/`, including:

- `config.json` — saved provider and budget preferences
- `credentials.json` — saved API keys
- `auth.json` — ChatGPT OAuth credentials
- `sessions/` — saved sessions for resume, search, and replay
- `memory/` — project-scoped persistent memory
- `missions.db` — durable mission store
- `automations.json` — automation config

On startup, Golem also looks for project instructions in places such as:

- `GOLEM.md`
- `CLAUDE.md`
- `.golem/instructions.md`
- `~/.golem/instructions.md`

## Where to go next

- **Feature guide:** [`docs/features.md`](docs/features.md)
- **Documentation/source-of-truth audit:** [`docs/documentation-audit.md`](docs/documentation-audit.md)
- **Mission orchestration details:** [`docs/mission-orchestration-prd.md`](docs/mission-orchestration-prd.md)

## Development

Build the project:

```bash
go build ./...
```

Run the test suite:

```bash
go test ./...
```

## Contributing

Issues and pull requests are welcome. If you change user-visible behavior, keep documentation aligned with the shipped command surfaces and keep `go test ./...` green.

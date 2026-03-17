# Golem

**Golem is a terminal-native coding agent for real repositories** — a local-first chat and mission workflow that starts in your shell, understands repo context on launch, and keeps sessions, memory, and durable mission state on your machine.

It is built for developers who want an agent that feels like part of the terminal workflow instead of a browser-heavy IDE.

## Why Golem

Golem front-loads the things that matter in day-to-day repo work:

- **Terminal-native by default** — launch a TUI, type prompts, use slash commands, and stay in the shell.
- **Repo-aware on startup** — Golem reads git state, project instructions, runtime config, saved sessions, and local memory before helping.
- **Local-first state** — sessions, auth, memory, missions, and automation config live under `~/.golem/`.
- **Operator-friendly UX** — `/help`, `/doctor`, runtime summaries, cost tracking, verification state, and explicit mission controls are shipped surfaces.
- **More than one-shot chat** — search earlier sessions, replay traces, rewind checkpoints, and manage bigger work through durable missions.

## What you can do with Golem

With the shipped CLI and TUI, you can:

- work interactively in a repository with an agent that can read, search, edit, write, list, and run commands,
- check setup and runtime quickly with `golem status`, `golem runtime`, and `/doctor`,
- recover prior work with `/resume`, `/search <query>`, `/replay`, and `/rewind`,
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

Prefer a prebuilt binary? Download the macOS Apple Silicon (`darwin-arm64`) or Linux x86_64 (`linux-amd64`) archive from this repository’s GitHub Releases page.

## Login and provider setup

### Fastest path: `golem login`

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

If you prefer environment variables, Golem also auto-detects credentials at runtime.

### Runtime providers

Shipped runtime paths support:

- **Anthropic**
- **OpenAI**
- **OpenAI-compatible / xAI**
- **Vertex AI**
- **Vertex AI Anthropic**

Important distinction: `golem login` supports `chatgpt`, `anthropic`, `openai`, and `xai`. Vertex providers are configured through environment variables.

### Useful auth and config environment variables

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

If you want the shortest path from install to a useful first run, do this inside a repository.

### 1. Log in

```bash
golem login
```

### 2. Confirm your runtime before launch

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
| `golem logout` | Remove saved local auth and provider config files |
| `golem status [--json]` | Show a one-shot status summary |
| `golem runtime [--json]` | Show the effective runtime profile |
| `golem dashboard [mission-id]` | Open Mission Control |
| `golem automations [list]` | List configured automations |
| `golem automations start` | Start the automation daemon |
| `golem automations status` | Show automation daemon status |
| `golem automations init` | Print an example automations config |

`golem logout` removes `~/.golem/config.json`, `~/.golem/credentials.json`, and `~/.golem/auth.json`, but does not clear environment variables or other local state such as sessions, memory, missions, or automations.

## Useful slash commands in the TUI

| Command | What it does |
| --- | --- |
| `/help` | Show commands and keybindings |
| `/clear` | Clear the current transcript |
| `/plan` | Show tracked plan progress |
| `/invariants` | Show the invariant checklist |
| `/runtime` | Show the effective runtime profile |
| `/verify` | Show the latest verification summary |
| `/compact` | Compress conversation context |
| `/cost` | Show session cost summary |
| `/budget` | Show budget status |
| `/resume` | Restore the last saved session |
| `/search <query>` | Search across saved sessions |
| `/model [name]` | Show or switch the active model |
| `/diff` | Show uncommitted git diff |
| `/undo [path]` | Revert one unstaged git-tracked change |
| `/replay [file\|list]` | Replay a recorded session trace |
| `/rewind [N]` | Rewind to a checkpoint or list checkpoints |
| `/doctor` | Diagnose setup issues |
| `/config` | Show effective configuration |
| `/team` | Show team member status |
| `/context` | Show context usage |
| `/skills` | List detected skills |
| `/skill <name>` | Toggle a skill |
| `/spec [file]` | Start or show a spec workflow |
| `/mission new <goal>` | Create a new durable mission |
| `/mission status` | Show the active mission summary |
| `/mission tasks` | List tasks in the active mission |
| `/mission plan` | Generate a task DAG for the active mission |
| `/mission approve` | Approve the pending mission plan |
| `/mission start` | Start mission execution |
| `/mission pause` | Pause the active mission |
| `/mission cancel` | Cancel the active mission |
| `/mission retry [task-id]` | Retry failed or blocked tasks, or one task by ID |
| `/mission list` | List known missions |
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
/mission retry
/mission retry task-123
/mission pause
/mission cancel
/mission list
```

What that flow looks like in practice:

- `/mission new <goal>` creates a durable mission in draft state
- `/mission plan` generates the task DAG
- `/mission approve` approves the pending plan
- `/mission start` starts execution when approvals allow it
- `/mission retry [task-id]` retries failed or blocked tasks, for all eligible tasks or for one task ID
- `golem dashboard` opens Mission Control for durable visibility into mission state

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
- `automations.json` — local automation configuration

Golem discovers project instructions from:

1. `GOLEM.md` in the working directory
2. `CLAUDE.md` in the working directory
3. `.golem/instructions.md` in the working directory
4. `~/.golem/instructions.md` for global instructions

Instructions from parent directories up to the git root are also loaded.

## Where to go next

- **Feature reference:** [`docs/features.md`](docs/features.md)
- **Mission behavior and orchestration details:** [`docs/mission-orchestration-prd.md`](docs/mission-orchestration-prd.md)
- **Spec workflow reference:** [`docs/spec-acceptance-checklist.md`](docs/spec-acceptance-checklist.md)
- **Repository entrypoint:** [`main.go`](main.go)

## Typical first-week workflows

### Fix something in a repo

```bash
golem
```

Then in the TUI:

```text
/help
/doctor
Summarize this repo and identify the riskiest area.
```

### Recover an earlier fix

```text
/search loader bug
/replay list
/rewind
```

### Start a larger multi-step task

```text
/mission new Stabilize the flaky end-to-end suite
/mission plan
/mission approve
/mission start
```

### Check runtime and cost

```bash
golem status
golem runtime
```

Then in the TUI:

```text
/runtime
/cost
/budget
```

## Notes on shipped surfaces

This README intentionally documents the currently shipped CLI and slash-command surfaces only.

In particular:

- durable mission control is available through `/mission ...` in the main TUI and `golem dashboard`
- `golem automations ...` is the shipped automation CLI family
- `/search <query>` is the supported search entry point for saved sessions
- mission retry is available as `/mission retry [task-id]`

## License

See the repository for license details.

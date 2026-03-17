# Command Reference

This page is the authoritative reference for Golem's shipped command surfaces.

It covers:

- shell commands implemented in `main.go`
- slash commands documented by the TUI help in `internal/ui/commands.go`
- the relationship between in-app mission workflows, `golem dashboard`, and the one-shot `status` / `runtime` reporting commands

If a command is not listed here, do not assume it is shipped.

## Surface map

Golem exposes three distinct operator-facing command surfaces:

1. **Shell commands** — one-shot terminal commands such as `golem status` and `golem dashboard`
2. **TUI slash commands** — commands entered inside the interactive app, such as `/help` and `/mission status`
3. **Mission Control** — the dedicated dashboard opened with `golem dashboard`

Important: mission lifecycle control is currently a **TUI slash-command workflow** plus `golem dashboard`. References to `golem mission ...` in older copy should be treated as aspirational unless that CLI family is actually implemented.

## Shell commands

These commands are implemented in `main.go`.

### `golem`

**Usage**

```bash
golem
golem <initial prompt...>
```

**Intent**

Launch the interactive TUI. If extra arguments are provided, Golem joins them into the initial prompt and starts the TUI with that prompt prefilled as the first request.

**Expected output**

- Starts the full terminal UI
- Shows the main chat shell, transcript, input box, and status surfaces
- Does not print a one-shot summary and exit

### `golem login`

**Usage**

```bash
golem login
golem login chatgpt
golem login anthropic
golem login openai
golem login xai
```

**Intent**

Run provider login interactively. If a provider argument is supplied, Golem targets that provider directly.

**Expected output**

- Performs an interactive login flow
- Writes provider/auth state under `~/.golem/`
- Prints `login error: ...` and exits non-zero on failure

### `golem logout`

**Usage**

```bash
golem logout
```

**Intent**

Remove saved local auth/config material managed by Golem.

**Expected output**

- Deletes saved local login state if present
- Prints `logout error: ...` and exits non-zero on failure
- Does not claim to clear environment-variable credentials

### `golem status`

**Usage**

```bash
golem status
golem status --json
```

**Intent**

Run a one-shot status check for the current configuration and runtime preparation state.

**Expected output**

- Human-readable status summary by default
- JSON report with `--json`
- Includes validation/runtime results rather than launching the TUI
- Exits non-zero if config validation fails or runtime preparation fails

**Relationship to the TUI**

- `golem status` is the shell-level quick check
- `/runtime` inside the TUI is the in-app view of the same runtime/profile family

### `golem runtime`

**Usage**

```bash
golem runtime
golem runtime --json
```

**Intent**

Print a richer one-shot runtime profile than `golem status`.

**Expected output**

- Human-readable runtime profile by default
- JSON report with `--json`
- Includes config validation and runtime-preparation details
- Exits non-zero if validation or runtime setup fails

**Relationship to the TUI**

- `golem runtime` is the shell entrypoint for runtime inspection
- `/runtime` is the interactive in-app rendering of runtime state

### `golem dashboard`

**Usage**

```bash
golem dashboard
golem dashboard <mission-id>
```

**Intent**

Open Mission Control, the dedicated dashboard for durable mission state.

**Expected output**

- Launches the Mission Control UI rather than the main chat shell
- If no mission ID is provided, selects the most relevant mission from durable state
- Can open into a valid empty state when no mission exists
- Prints `dashboard error: ...` and exits non-zero if startup fails

**Relationship to mission commands**

- `golem dashboard` is the shell entrypoint to mission inspection
- mission lifecycle actions are still primarily driven by `/mission ...` inside the main TUI

### `golem automations`

**Usage**

```bash
golem automations
golem automations list
golem automations start
golem automations status
golem automations init
```

**Intent**

Inspect or run local automation configuration.

**Expected output**

- `golem automations` defaults to `list`
- `list` prints configured automations
- `start` launches the automation daemon in the foreground
- `status` prints a daemon status summary
- `init` prints an example `~/.golem/automations.json`
- Unknown subcommands print usage and exit non-zero

## Slash commands in the TUI

These commands are documented by the in-app help generated in `internal/ui/commands.go` and are entered inside the interactive `golem` app.

| Command | Usage | Intent / expected output |
| --- | --- | --- |
| `/help` | `/help` | Shows the built-in command list, discoverability guidance, and key hints. |
| `/clear` | `/clear` | Clears the current transcript in the active TUI session. |
| `/plan` | `/plan` | Summarizes the current tracked plan, or explains that no plan exists yet. |
| `/invariants` | `/invariants` | Summarizes the tracked invariant checklist with hard/soft status counts. |
| `/runtime` | `/runtime` | Shows the effective runtime profile inside the TUI. |
| `/verify` | `/verify` | Shows the latest verification summary, or reports that no verification is recorded yet. |
| `/compact` | `/compact` | Compresses conversation context to recover space in the current session. |
| `/cost` | `/cost` | Shows per-session usage and cost breakdown. |
| `/budget` | `/budget` | Shows current budget status, thresholds, and fallback-model information. |
| `/resume` | `/resume` | Restores the last saved session if one exists. |
| `/search <query>` | `/search <query>` | Searches saved sessions. When called without a query, Golem shows usage text and examples. |
| `/model [name]` | `/model` or `/model <name>` | Shows the active model, or switches the next run to a different model. |
| `/diff` | `/diff` | Shows a git diff summary of uncommitted changes. |
| `/undo [path]` | `/undo` or `/undo <path>` | Reverts unstaged git-tracked changes, either for all eligible files or for a specific path. |
| `/replay [file\|list]` | `/replay`, `/replay list`, or `/replay <file>` | Lists saved session traces or replays a selected trace. |
| `/rewind [N]` | `/rewind` or `/rewind <N>` | Lists checkpoints or rewinds the session to a selected checkpoint. |
| `/doctor` | `/doctor` | Diagnoses setup state and highlights likely configuration or auth issues. |
| `/config` | `/config` | Shows the effective configuration seen by the running app. |
| `/team` | `/team` | Shows team-mode / teammate status if present, or an empty-state message otherwise. |
| `/context` | `/context` | Shows current context window usage and compacting guidance. |
| `/skills` | `/skills` | Lists detected skills. |
| `/skill <name>` | `/skill <name>` | Toggles or activates a named skill in the current session. |
| `/spec [file]` | `/spec` or `/spec <file>` | Shows the current spec workflow state, or starts/specifies a spec-driven workflow target. |
| `/mission [new\|status\|tasks\|plan\|approve\|start\|pause\|cancel\|list]` | `/mission ...` | Runs the shipped TUI mission workflow. See the mission section below for semantics. |
| `/quit` or `/exit` | `/quit` or `/exit` | Quits the app. |

## Mission workflow reference

The main TUI help intentionally documents the mission workflow as:

```text
/mission [new|status|tasks|plan|approve|start|pause|cancel|list]
```

This is the shipped mission control surface to document for operators.

### Mission lifecycle semantics

These semantics align with `docs/features.md`:

- `/mission new <goal>` creates a durable mission in `draft` state.
- `/mission status` summarizes durable mission state, including current phase, next action, approvals, and task progress.
- `/mission tasks` lists the current task DAG when one exists.
- `/mission plan` is the normal path from `draft` to a planned task graph. Applying the plan moves the mission to `awaiting_approval` and creates a durable mission-plan approval gate.
- `/mission approve` resolves that durable plan approval and immediately attempts to start execution. If another approval still blocks execution, the UI should say so explicitly.
- `/mission start` does **not** bypass approval. It starts a mission from `paused`, or from `awaiting_approval` only after the plan approval is already approved and no pending approvals remain.
- `/mission pause` stops new task leasing by pausing the in-process orchestrator.
- `/mission cancel` stops the orchestrator, marks the mission cancelled, and clears the active mission from the current chat session.
- `/mission list` lists known missions and marks the current active mission in the TUI session.
- Resume semantics are `/mission start`; there is no separate shipped `/mission resume` slash command.

### Relationship to Mission Control

`/mission ...` and `golem dashboard` operate on the same durable mission state, but they serve different operator needs:

- use `/mission ...` in the main chat TUI to create, plan, approve, start, pause, cancel, and inspect the active mission
- use `golem dashboard` to inspect durable mission state outside the chat transcript
- expect Mission Control to render the header plus **Tasks**, **Workers**, **Evidence**, and **Events** panes
- expect the dashboard to open into a valid empty state if no mission exists

Important: docs should not imply a shipped `golem mission ...` CLI family unless such a CLI is actually implemented. Today, the shell mission entrypoint is `golem dashboard`; mission lifecycle control is primarily in the TUI.

## Runtime, status, and discoverability

These command surfaces are related but not interchangeable:

- `golem status` is a one-shot shell summary for config/runtime readiness
- `golem runtime` is a richer one-shot shell runtime profile
- `/runtime` is the in-app runtime/profile view
- `/help` is the discoverability anchor inside the TUI
- `/search <query>` is the recovery command for finding saved sessions and prior work
- `golem dashboard` is the separate Mission Control surface, not a substitute for the main chat shell

## Keybindings called out by help

The TUI help also calls out these operator shortcuts:

| Key | Intent |
| --- | --- |
| `Enter` | Send the current message |
| `Shift+Enter` | Insert a newline |
| `Tab` | Autocomplete slash commands |
| `Esc` | Cancel the active run |
| `Ctrl+L` | Clear the transcript |
| `↑/↓` | Recall input history |
| `PgUp/PgDn` | Scroll the transcript |

## Source of truth

Use these implementation files when updating this page:

- `main.go` for shell commands
- `internal/ui/commands.go` for top-level slash-command help and usage intent
- `docs/features.md` for mission workflow semantics and Mission Control behavior

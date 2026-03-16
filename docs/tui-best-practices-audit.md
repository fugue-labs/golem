# TUI Best Practices Audit

## Purpose

This audit establishes the execution-ready non-regression baseline for the shipped Golem TUI before visual or behavioral implementation changes begin.

It maps the broad "best coding agent TUI" goal onto the repo's current code layout, current user-visible surfaces, and the end-to-end contract preserved by `test/e2e/tuistory_test.go`.

### Sources reviewed

- `main.go`
- `internal/ui/app.go`
- `internal/ui/commands.go`
- `internal/ui/chat/messages.go`
- `internal/ui/workflow_panel.go`
- `internal/ui/dashboard/dashboard.go`
- `internal/ui/styles/styles.go`
- `docs/features.md`
- `docs/implementation-spec.md`
- `test/e2e/tuistory_test.go`

## Non-regression rule

This audit is advisory for prioritization, but the shipped operator contract is not optional. Any implementation work must preserve the behavior already exercised by `test/e2e/tuistory_test.go` unless docs, tests, and code are updated together.

## Target shipped surfaces

These are the five TUI surfaces that current implementation work must use as the scoring rubric and execution map.

### 1. Shell

**Primary code paths**
- `main.go`
- `internal/ui/app.go`
- `internal/ui/styles/styles.go`

**What ships today**
- `main.go` launches the main Bubble Tea shell with `ui.New(cfg)` and `tea.NewProgram(m)`.
- `internal/ui/app.go` owns the main shell frame, startup behavior, window title, focus handling, minimum-size fallback, compact fallback, header, transcript region, input region, and status bar.
- The launch frame keeps a visible `GOLEM` anchor and a visible input affordance (`❯` or `Ask anything… /help for commands`).
- The shell remains responsive through cancellation, paging, input history, slash commands, and file watcher updates.

**Why it matters first**
- This is the entry point for default `golem` usage.
- It carries the strongest e2e non-regression contract.
- It is the highest-leverage place for clarity, stability, and perceived polish.

### 2. Transcript

**Primary code paths**
- `internal/ui/app.go`
- `internal/ui/chat/messages.go`

**What ships today**
- `internal/ui/app.go` routes user input, busy state, scroll position, queueing, cancel flow, and transcript composition.
- `internal/ui/chat/messages.go` renders user, assistant, thinking, tool call, tool result, summary, and error states.
- The transcript supports paging and resilient visibility of sent messages, slash-command output, tool activity, and errors.

**Why it matters second**
- It is where almost all agent interaction becomes operator-visible.
- It is the surface affected by `/help`, `/search <query>`, unknown-command errors, `/clear`, `/model`, `/doctor`, `/cost`, replay/rewind guidance, and agent cancellation outcomes.

### 3. Workflow rail

**Primary code paths**
- `internal/ui/workflow_panel.go`
- supporting state from `internal/ui/app.go`

**What ships today**
- The workflow rail appears conditionally based on shell width and available workflow state.
- It summarizes mission, spec, plan, verification, invariants, and team state.
- On narrower terminals, the shell falls back to stacked or shell-summary behavior instead of removing workflow context entirely.

**Why it matters third**
- It is the current home of continuous progress visibility.
- It should remain secondary to the transcript but must stay aligned with operator priorities: active work, blockers, approvals, verification, and next action.

### 4. Dashboard

**Primary code paths**
- `main.go`
- `internal/ui/dashboard/dashboard.go`
- `internal/ui/styles/styles.go`

**What ships today**
- `main.go` dispatches the `dashboard` subcommand into `runDashboard()`.
- `internal/ui/dashboard/dashboard.go` renders Mission Control, durable mission selection, header metrics, and the four panes: Tasks, Workers, Evidence, and Events.
- The dashboard supports pane navigation with `Tab`, `Shift+Tab`, `1-4`, `j/k`, `r`, and `q`.
- The empty state remains valid when no mission exists.

**Why it matters fourth**
- It is the second major shipped operator surface.
- It is the durable mission-control view that must stay consistent with mission state independent of the transcript.

### 5. Discoverability

**Primary code paths**
- `internal/ui/app.go`
- `internal/ui/commands.go`
- `internal/ui/dashboard/dashboard.go`

**What ships today**
- `/help` renders the canonical command and keybinding index.
- Slash-command completion is exposed through `Tab` in the main shell.
- Launch guidance, placeholder text, status-bar hints, welcome copy, and dashboard footer hints all reinforce next actions.
- `/search <query>` usage copy is part of the preserved shell contract.

**Why it matters continuously**
- Discoverability is not a single panel; it is distributed across launch, empty states, command help, and status chrome.
- It is the main protection against a "powerful but opaque" TUI.

## Preserved e2e contract to keep linked to implementation work

Every TUI implementation task should explicitly cite which of these preserved behaviors it must not regress.

| Contract item | Current implementation anchor | E2E evidence |
|---|---|---|
| Visible `GOLEM` launch anchor | `internal/ui/app.go` header/status rendering | `TestTuistoryLaunchAndMessage` |
| Visible input affordance on launch | `internal/ui/app.go` placeholder, prompt, input rendering | `TestTuistoryLaunchAndMessage` |
| `/help` shows command list and key hints | `internal/ui/commands.go: renderHelpMessage`, input handling in `app.go` | `TestTuistoryHelpCommand` |
| `/search <query>` usage copy stays explicit | `internal/ui/commands.go: handleSearchCommand` | `TestTuistorySearchFlow` |
| Mission dashboard launches into valid Mission Control or empty/error state | `main.go`, `internal/ui/dashboard/dashboard.go` | `TestTuistoryDashboard` |
| Dashboard pane navigation remains intact | `internal/ui/dashboard/dashboard.go` key handling | `TestTuistoryDashboard` |
| Unknown slash commands fail obviously | `internal/ui/app.go` unknown-command branch | `TestTuistoryUnknownCommand` |
| `Esc` cancels active work without bricking shell responsiveness | `internal/ui/app.go`, `internal/ui/workflow_panel.go` cancellation helpers | `TestTuistoryEscCancellation` |
| `PgUp/PgDn` transcript paging remains stable | `internal/ui/app.go` key handling and transcript scroll | `TestTuistoryPageUpDown` |
| `↑/↓` input history remains stable | `internal/ui/app.go` input-history handling | `TestTuistoryInputHistory` |
| `/clear`, `/model`, `/doctor`, `/cost`, replay/rewind, tab completion remain stable | `internal/ui/app.go`, `internal/ui/commands.go` | `TestTuistoryClearCommand`, `TestTuistoryModelCommand`, `TestTuistoryDoctorCommand`, `TestTuistoryCostCommand`, `TestTuistoryReplayFlow`, `TestTuistoryCheckpointRewindFlow`, `TestTuistoryTabCompletion`, `TestTuistorySlashCommandSequence` |
| File watcher updates do not destabilize shell identity | `internal/ui/app.go` file-watcher startup and event handling | `TestTuistoryFileWatcher` |

## Best-practices scoring rubric mapped to shipped surfaces

| Surface | Current shipped baseline | What must improve without breaking contract |
|---|---|---|
| Shell | Strong launch identity, minimum-size fallback, status framing, input visibility, `GOLEM` anchor | Improve clarity and hierarchy without hiding transcript/input/status behavior |
| Transcript | Strong role separation and tool rendering, manual paging, resilient slash-command output | Improve scanability and density without changing asserted text or scroll/cancel/history behavior |
| Workflow rail | Good continuous workflow summary and width-aware presentation | Make active work and next action clearer without overwhelming transcript |
| Dashboard | Strong Mission Control identity, four-pane model, valid empty state, keyboard navigation | Improve scanability and emphasis without changing pane model or navigation contract |
| Discoverability | `/help`, placeholder, welcome/status hints, tab completion, dashboard hints | Keep teaching next actions from launch onward without making the shell noisy |

## Prioritized execution order aligned to current code layout

This is the order implementation work should follow so changes track the repo's actual architecture.

### P0 — Baseline-preserving shell entry and layout framing
1. `main.go`
   - preserve launch routing between default shell and `dashboard`
   - keep `GOLEM`-visible startup behavior stable
2. `internal/ui/app.go`
   - preserve shell frame, transcript region, input region, status bar, cancellation, paging, and history
3. `internal/ui/styles/styles.go`
   - adjust appearance only after shell structure remains contract-safe

### P1 — Discoverability and command contract
1. `internal/ui/commands.go`
   - preserve `/help` output, `/search <query>` usage copy, `/doctor`, `/model`, `/cost`, replay/rewind guidance
2. `internal/ui/app.go`
   - preserve slash-command dispatch, unknown-command handling, tab completion, input/history behavior

### P2 — Transcript readability
1. `internal/ui/chat/messages.go`
   - refine message rhythm, tool rendering, and summary hierarchy
2. `internal/ui/app.go`
   - keep transcript scroll behavior, viewport budgeting, and user-visible send/cancel results stable

### P3 — Workflow visibility
1. `internal/ui/workflow_panel.go`
   - improve the workflow rail only after shell and transcript contracts are protected
2. `internal/ui/app.go`
   - keep width gating and stacked layout behavior aligned with shell layout rules

### P4 — Dashboard readability
1. `main.go`
   - preserve `dashboard` subcommand routing
2. `internal/ui/dashboard/dashboard.go`
   - improve Mission Control readability while preserving pane model, empty state, and navigation
3. `internal/ui/styles/styles.go`
   - align visual treatment with shell changes after functional behavior remains intact

## Highest-value code paths before implementation changes

If implementation work has to start immediately, review and protect these paths first:

1. `main.go: run()` — top-level routing into shell vs dashboard
2. `internal/ui/app.go: New(), Init(), Update(), View()` — shell lifecycle
3. `internal/ui/app.go: handleKey()` — slash commands, cancellation, paging, history, tab completion
4. `internal/ui/commands.go: renderHelpMessage(), handleSearchCommand()` — preserved help/search copy
5. `internal/ui/chat/messages.go` — transcript role rendering and tool output formatting
6. `internal/ui/workflow_panel.go` — workflow rail layout and prioritization
7. `internal/ui/dashboard/dashboard.go` — Mission Control rendering and pane navigation
8. `internal/ui/styles/styles.go` — cross-surface visual consistency

## Implementation guardrail

Before changing the TUI, each task should name:

1. the target surface: shell, transcript, workflow rail, dashboard, or discoverability,
2. the exact preserved e2e contract items it must keep,
3. the primary implementation files being touched in `main.go` or `internal/ui`, and
4. the execution phase above that justifies the order of work.

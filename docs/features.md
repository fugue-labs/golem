# Golem Features

Golem is a terminal-based coding agent with deep project awareness and a rich interactive experience. This document covers all user-facing features.

## TUI superiority rubric and preserved UX contract

Use this section as the shared scorecard for shell-oriented implementation work. It is derived from the current docs, UI code, and e2e-observed behavior so future improvements can reference one contract instead of re-deriving goals from scattered sources.

### Current TUI experience

The current interface is organized around five user-visible regions that any future polish should respect:

- **Shell layout** — a two-line header, central transcript, always-visible input, and persistent `GOLEM` status bar.
- **Chat rendering** — prompt-led user messages, markdown assistant responses, inline tool activity, and lightweight system summaries.
- **Workflow panel** — a conditional right rail that appears on wider terminals to show mission, spec, team, plan, invariants, and verification state.
- **Dashboard** — a separate `golem dashboard` Mission Control surface for tasks, workers, evidence, and events.
- **Discoverability affordances** — `/help`, slash-command tab completion, launch-time tips, placeholder guidance, and status-bar key hints.

### Superiority rubric by surface

| Surface | What success means | Score 1 | Score 3 | Score 5 |
|---|---|---|---|---|
| Launch frame | The first frame makes identity, orientation, and next action obvious. | Looks unstable or under-explained. | `GOLEM`, input affordance, and basic shell regions are visible. | The shell feels immediately intentional, legible, and ready for action. |
| Transcript readability | User, assistant, tool, system, and error turns are easy to scan over time. | Turns blur together. | Main roles are distinguishable but still dense. | Role separation, streaming state, and summaries are obvious at a glance. |
| Workflow visibility | Active mission/workflow state is prioritized over raw detail. | State exists but is hard to rank. | Current work is visible with some effort. | Active work, blockers, next action, and proof surfaces are immediately clear. |
| Dashboard readability | Mission Control reads clearly before the operator starts navigating. | Panes feel cramped or hard to parse. | Pane purpose and focus are understandable after brief inspection. | Pane headers, summaries, empty states, and focus affordances are obvious immediately. |
| Discoverability | Help, slash commands, usage text, and key hints are easy to find from launch onward. | Requires prior product knowledge. | `/help` and some hints are visible. | The shell continuously teaches next actions without becoming noisy. |

### How to prioritize improvements

1. **Clarify shell hierarchy first**
   - Make the header, transcript, workflow panel, and status bar read as one cohesive layout.
   - Favor spacing, labels, and section emphasis over major structural churn.
2. **Make chat states easier to scan**
   - Increase separation between user turns, assistant output, tool activity, errors, and summaries.
   - Preserve markdown quality while making long-running sessions easier to parse quickly.
3. **Refocus the workflow panel on active work**
   - Surface the current bottleneck, blocked state, next action, and in-progress work before secondary detail.
   - Keep dense supporting state available without letting it dominate the rail.
4. **Improve dashboard readability before extending scope**
   - Strengthen Mission Control pane headers, summary metrics, focus indicators, and empty states.
   - Prioritize first-glance comprehension over adding more dashboard controls.
5. **Distribute discoverability across the shell**
   - Keep `/help` as the canonical command index, but reinforce it with stronger empty-state cues, slash affordances, and status hints.

### Preserved e2e UX contract

These behaviors are verified by end-to-end tests and must remain stable through visual polish:

- **Visible `GOLEM` launch anchor** — the shell must render with a visible `GOLEM` status bar and visible prompt/input affordance (`❯` or `Ask anything… /help for commands`).
- **`/help` discoverability** — `/help` must continue surfacing `/help`, `/clear`, `/plan`, `/model`, `/cost`, `/replay`, `/search`, `/rewind`, `/mission`, `/quit`, `/spec`, and `/doctor`, plus the key hints `Enter`, `Esc`, and `Tab`.
- **`/search` usage copy** — `/search` without arguments must continue showing `/search <query>`, `search across all saved sessions`, and `Examples`.
- **Dashboard launch stability** — `golem dashboard` must continue to land in either `Mission Control` or a valid no-mission/error state while preserving pane navigation.
- **Cancellation, scroll, and history behavior** — `Esc` cancellation, `PgUp/PgDn` transcript scrolling, and `↑/↓` input history recall must remain stable and readable.
- **Other shell resilience** — unknown slash commands must still fail obviously; `/clear`, `/model`, `/doctor`, tab completion, and slash-command sequencing must remain intact.

### Task template for future TUI work

Implementation tasks should reference:

1. the target **surface** from the rubric,
2. the current and desired **score** for that surface,
3. the preserved **e2e UX contract** bullets that cannot regress, and
4. any additional mission/dashboard context from `docs/tui-comparison-prompt.md`.

## Commands

| Command | Description |
|---------|-------------|
| `/help` | Show all commands and keybindings |
| `/clear` | Clear the conversation transcript |
| `/plan` | Show the current tracked plan with progress |
| `/invariants` | Show the invariant checklist with pass/fail status |
| `/runtime` | Show the effective runtime profile |
| `/verify` | Show the latest verification summary |
| `/compact` | Compress conversation context to free up space |
| `/cost` | Show detailed session cost breakdown |
| `/replay [file\|list]` | Replay a recorded session trace |
| `/budget` | Show budget status and limits |
| `/resume` | Restore the last saved session |
| `/search <query>` | Search across all saved sessions |
| `/model [name]` | Show or switch the active model |
| `/diff` | Show git diff of uncommitted changes |
| `/undo [path]` | Revert unstaged changes (all files, or a specific path) |
| `/mission [new\|status\|tasks\|plan\|approve\|start\|pause\|cancel\|list]` | Mission orchestration commands |
| `/rewind [N]` | Rewind to a saved checkpoint or list checkpoints |
| `/doctor` | Diagnose setup issues |
| `/config` | Show effective configuration and environment variables |
| `/team` | Show team member status |
| `/context` | Show context window usage |
| `/skills` | List detected skills |
| `/skill <name>` | Toggle a skill on or off |
| `/spec [file]` | Start or show spec-driven development |
| `/quit` or `/exit` | Quit the app |

## Keybindings

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Shift+Enter` | Insert newline |
| `Esc` | Cancel the active run |
| `Up/Down` | Recall input history (when idle) |
| `PgUp/PgDn` | Scroll the transcript |

## Status Bar

The status bar shows real-time information:

- **Provider/Model** — current LLM provider and model
- **Tokens** — input/output token counts for the last request
- **Cache** — cache read/write token counts (when applicable)
- **Tools** — number of tool calls in the last request
- **Plan** — plan progress (completed/total)
- **Invariants** — hard constraint pass/fail/unknown counts
- **Verify** — verification badge (pass/fail/stale)
- **Session** — cumulative session tokens (input/output)
- **Context** — context window usage percentage
- **Cost** — cumulative session cost
- **Scroll** — scroll offset when scrolling
- **Queued** — number of queued user messages

## Per-Request Usage Summary

After each agent run, a muted summary line shows:
- Token counts (input/output)
- Tool call count
- Elapsed time
- Cost for that request
- Context window usage %

When context usage exceeds 80%, a warning appears suggesting `/compact`.

## Project Instructions

Golem discovers and loads project-specific instructions from:
1. `GOLEM.md` in the working directory
2. `CLAUDE.md` in the working directory (Claude Code compatibility)
3. `.golem/instructions.md` in the working directory
4. `~/.golem/instructions.md` for global instructions

Instructions from parent directories up to the git root are also loaded.

## Git Awareness

On startup, Golem detects the git repository and captures:
- Current branch name
- Clean/dirty status
- Recent commits

This context is injected into the system prompt and shown in the header.

## Session Persistence

Sessions are auto-saved after each successful run to `~/.golem/sessions/`. Use `/resume` to restore the last session, preserving conversation history, tool state, plan state, and verification state.

## Auto Memory

Golem maintains persistent memory per project using an SQLite store at `~/.golem/memory/<project-hash>/`. The agent can save and recall facts, patterns, and project knowledge across sessions. Relevant memories are automatically injected into context.

## Cost Tracking

Real-time cost estimation based on model pricing. Tracks per-model cost breakdown, cumulative session cost, and shows cost in the status bar. Use `/cost` for a detailed breakdown.

## Tool Approval

Configurable permission modes:
- **auto** — approve all tool calls automatically
- **suggest** — show tool calls and ask for approval before mutating operations
- **plan** — read-only until a plan is approved

When in `suggest` mode, mutating tools (edit, write, bash) show an approval prompt. Press `y` to approve, `n` to deny, `a` to always allow, or `d` to always deny for that tool.

## MCP Server Support

Connect to MCP (Model Context Protocol) servers for extended capabilities. Configure in `~/.golem/mcp.json`:

```json
{
  "servers": [
    {
      "name": "browser",
      "command": "npx",
      "args": ["-y", "@anthropic/mcp-browser"]
    }
  ]
}
```

## Shell Hooks

Run shell commands before/after tool execution. Configure in `~/.golem/hooks.json`:

```json
{
  "pre_tool_use": [
    {
      "command": "echo 'Running tool: $GOLEM_TOOL'",
      "tools": ["bash"],
      "timeout": 5
    }
  ],
  "post_tool_use": [
    {
      "command": "notify-send 'Tool done: $GOLEM_TOOL'"
    }
  ]
}
```

Hooks receive `GOLEM_TOOL` (tool name) and `GOLEM_DATA` (JSON args) environment variables.

## Skills

Skills are markdown files in `~/.claude/skills/` that extend the agent's capabilities. Use `/skills` to list available skills and `/skill <name>` to toggle one on or off.

## Image Support

The view tool supports reading image files (PNG, JPG, GIF, WEBP, SVG) and sends them as multimodal content for visual analysis.

## Bell Notification

When an agent task takes more than 5 seconds, the terminal bell rings on completion.

## Input History

Arrow up/down recalls previously entered prompts when the agent is idle.

## Context Window Warning

When context usage exceeds 80%, a warning appears suggesting to run `/compact` to free up space.

## Configuration

Set configuration via environment variables:

| Variable | Description |
|----------|-------------|
| `GOLEM_PROVIDER` | LLM provider (anthropic, openai, openai_compatible, vertexai, vertexai_anthropic) |
| `GOLEM_MODEL` | Model name |
| `GOLEM_API_KEY` | API key |
| `GOLEM_TIMEOUT` | Request timeout |
| `GOLEM_TEAM_MODE` | Team mode (auto, on, off) |
| `GOLEM_PERMISSION_MODE` | Permission mode (auto, suggest, plan) |
| `GOLEM_MCP_SERVERS` | MCP servers config path |

Use `/config` to see the effective configuration and `/doctor` to diagnose issues.

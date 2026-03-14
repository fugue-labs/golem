# Golem Features

Golem is a terminal-based coding agent with deep project awareness and a rich interactive experience. This document covers all user-facing features.

## Current TUI experience

The current interface is organized around five user-visible regions that any future polish should respect:

- **Shell layout** — a two-line header, central transcript, always-visible input, and persistent `GOLEM` status bar.
- **Chat rendering** — prompt-led user messages, markdown assistant responses, inline tool activity, and lightweight system summaries.
- **Workflow panel** — a conditional right rail that appears on wider terminals to show mission, spec, team, plan, invariants, and verification state.
- **Dashboard** — a separate `golem dashboard` Mission Control surface for tasks, workers, evidence, and events.
- **Discoverability affordances** — `/help`, slash-command tab completion, launch-time tips, placeholder guidance, and status-bar key hints.

## Highest-impact TUI improvements to pursue first

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

## TUI design constraints from e2e coverage

These constraints are currently verified by end-to-end tests and should be treated as non-negotiable when improving the interface:

- **Stable launch behavior** — the app must render reliably with a visible `GOLEM` status bar and visible prompt/input affordance.
- **Help and status discoverability must remain intact** — `/help` must continue exposing key commands and keybindings, and the shell must continue making controls easy to find.
- **Search usage copy must remain stable** — `/search` without arguments must continue showing `/search <query>` and clear examples for searching saved sessions.
- **Command resilience must remain obvious** — `/model`, `/doctor`, `/clear`, unknown slash commands, cancellation, scrolling, tab completion, and input history must remain visually understandable and responsive.
- **Dashboard launch must remain stable** — `golem dashboard` must continue to open into either Mission Control or a valid empty/error state without losing navigation affordances.

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
| `/resume` | Restore the last saved session |
| `/model [name]` | Show or switch the active model |
| `/diff` | Show git diff of uncommitted changes |
| `/undo [path]` | Revert unstaged changes (all files, or a specific path) |
| `/doctor` | Diagnose setup issues |
| `/config` | Show effective configuration and environment variables |
| `/skills` | List detected skills |
| `/skill <name>` | Toggle a skill on or off |
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

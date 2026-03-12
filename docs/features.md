# Golem Features

Golem is a terminal-based coding agent with deep project awareness and a rich interactive experience. This document covers all user-facing features.

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

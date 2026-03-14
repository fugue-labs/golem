# TUI Comparison & Improvement Prompt

Use this document to turn reference-TUI observations into a concrete rubric for improving golem's own interface. Two prompts below. **Prompt 1** runs once to build a feature catalog from the reference TUI. **Prompt 2** runs per-feature to compare and fix golem.

## Current golem baseline and improvement rubric

### Current baseline from the code

- **Shell layout** — the main TUI is a stable four-part shell: a two-line header, scrollable chat transcript, always-visible input area, and a bottom `GOLEM` status bar. On terminals `>=110` columns, a fixed-width workflow panel appears on the right.
- **Chat rendering** — user messages render with the `❯` prompt mark, assistant messages render as markdown, tool calls render as compact rows with inline results, and the busy state appears directly above the input with spinner/tool/queue details.
- **Workflow panel** — the right rail combines mission, spec, team, plan, invariants, and verification into a single dense panel. It is informative, but it currently optimizes for completeness over scanability.
- **Dashboard** — `golem dashboard` opens a separate “Mission Control” view with task, worker, evidence, and event panes plus keyboard navigation (`tab`, `shift+tab`, `1-4`, `j/k`, `q`, `r`).
- **Discoverability affordances** — the empty state rotates tips, the input placeholder says `Ask anything… /help for commands`, slash commands support tab completion, `/help` is the canonical command index, and the status bar always preserves a visible `GOLEM` anchor.

### Visual target

Treat the desired target as a **clearer, more opinionated shell** rather than a net-new layout:

1. **Stable launch frame** — the app should still open quickly into a recognizably stable shell with visible prompt and `GOLEM` status bar.
2. **At-a-glance hierarchy** — header = context, transcript = primary task, right rail/dashboard = workflow state, footer/status = controls and runtime state.
3. **Role separation in the transcript** — user, assistant, tool activity, errors, and summaries should be visually distinct without becoming noisy.
4. **Workflow readability over raw density** — the right rail and dashboard should emphasize the active section, next action, and blocked state before full detail.
5. **Persistent discoverability** — help, slash commands, status hints, and launch-time orientation must remain obvious even after visual upgrades.

### Improvement scoring rubric

Score candidate UX improvements on these four dimensions before implementation:

| Dimension | Question | Score 1 | Score 3 | Score 5 |
|---|---|---:|---:|---:|
| User impact | Does this improve first-run comprehension or frequent-task speed? | Niche polish | Noticeable | Core daily win |
| Discoverability | Does it make commands, status, or next steps easier to find? | Hidden | Somewhat clearer | Immediately obvious |
| Layout leverage | Does it improve the shared shell/chat/workflow structure already in code? | Local only | One region | Entire shell feels better |
| Test compatibility | Can it preserve current e2e-observed behavior with low risk? | High risk | Manageable | Safe/default-preserving |

Prioritize work scoring high in **user impact + discoverability + layout leverage**, while avoiding changes that lower **test compatibility**.

### Highest-impact TUI improvements to pursue first

1. **Strengthen the shell information architecture**
   - **Current gap:** the header, transcript, workflow panel, and status bar all work, but they do not yet feel like a single opinionated composition.
   - **Target:** tighten visual hierarchy with clearer region titles, spacing, and empty-state framing while keeping the launch behavior stable.
2. **Improve chat role separation and streaming readability**
   - **Current gap:** assistant markdown, tool activity, system summaries, and errors can blur together during long sessions.
   - **Target:** make user/assistant/tool/system states more immediately scannable through spacing, labels, muted metadata, and clearer in-progress treatment.
3. **Refocus the workflow panel on “what matters now”**
   - **Current gap:** the workflow panel surfaces a lot of state, but active work, blocked work, and next action are not strongly prioritized.
   - **Target:** bias the panel toward the current mission/spec/plan/verification bottleneck, with secondary sections visually de-emphasized or collapsed.
4. **Upgrade dashboard scanability before adding new dashboard features**
   - **Current gap:** Mission Control already has useful pane structure, but the task/worker/evidence/events composition is still cramped and operator-heavy.
   - **Target:** improve pane headers, summary metrics, empty states, and focus visibility so the dashboard reads well before interaction begins.
5. **Expand discoverability without weakening the existing command model**
   - **Current gap:** `/help`, tab completion, and status hints work, but novice guidance is still concentrated in help output rather than distributed through the shell.
   - **Target:** keep `/help` as source of truth while adding inline affordances such as stronger empty-state guidance, slash affordances, and more explicit status hints.

### Non-negotiable constraints from e2e coverage

These constraints must be preserved by any visual redesign or comparison task:

- **Stable launch behavior:** the initial TUI must render reliably with a visible `GOLEM` status bar and visible input affordance (`❯` prompt or `Ask anything…`).
- **Preserved help discoverability:** `/help` must continue surfacing key commands including `/help`, `/clear`, `/plan`, `/model`, `/cost`, `/replay`, `/search`, `/rewind`, `/mission`, `/quit`, `/spec`, and `/doctor`, plus keybindings including `Enter`, `Esc`, and `Tab`.
- **Preserved search usage text:** `/search` with no argument must continue showing `/search <query>`, the phrase `search across all saved sessions`, and `Examples`.
- **Preserved resilience affordances:** unknown slash commands must still produce an obvious error, `/model` and `/doctor` must remain easy to find and readable, `/clear` must still remove transcript content, and launch/cancel/scroll/history behaviors must stay stable.
- **Dashboard stability:** `golem dashboard` must continue to launch into either a valid `Mission Control` view or a valid no-mission/error state without regressing keyboard navigation affordances.

---

## Prompt 1: Feature Discovery (run once against the reference TUI)

```
You are a TUI reverse-engineer. Your job is to systematically explore a reference
terminal application and produce a structured feature catalog that another agent
will use to improve a competing TUI.

REFERENCE APPLICATION: droid
(or substitute: claude -- for Claude Code)

TOOLS: You have tuistory installed. Use it for all TUI interaction.

METHODOLOGY:

1. Launch the reference TUI:
   tuistory launch "droid" -s ref --cols 120 --rows 40

2. Take an initial snapshot and document the default state:
   tuistory -s ref snapshot --trim
   Record: layout structure, visible panels, status bar contents, prompt style,
   color usage, border characters, whitespace/padding.

3. Systematically test every interaction category below. For each, type the
   input, take a snapshot, and record what happened. If an interaction does
   nothing, record that too.

INTERACTION CATEGORIES TO EXPLORE:

A) INPUT BEHAVIOR
   - Type a simple message and press enter
   - Type a multi-line message (shift+enter or similar)
   - Use arrow keys in the input area
   - Paste a long block of text
   - Try input while agent is responding
   - Empty enter (no text)

B) KEYBOARD SHORTCUTS
   Test each of these and record what happens:
   - ctrl+c (during idle, during response)
   - ctrl+d
   - ctrl+l
   - ctrl+r
   - escape
   - tab
   - ctrl+a, ctrl+e, ctrl+k, ctrl+u (line editing)
   - ctrl+z
   - Any other key combos visible in help or status bar

C) SLASH COMMANDS
   Type "/" and take a snapshot to see autocomplete/menu.
   Try common commands: /help, /clear, /compact, /config, /status,
   /model, /cost, /quit, /exit, /logout, /login
   Record which exist and what they do.

D) RESPONSE RENDERING
   Ask the model to produce:
   - A markdown heading and paragraph
   - A code block with syntax highlighting
   - A bulleted list
   - A numbered list
   - A table
   - Bold, italic, inline code
   - A very long response (ask for a 500-word essay)
   Record how each renders: colors, indentation, wrapping, scroll behavior.

E) TOOL USE DISPLAY
   Ask the model to:
   - Read a file (observe how tool calls are displayed)
   - Edit a file (observe permission prompts, diff display)
   - Run a shell command (observe output rendering)
   - Search for something (observe search results display)
   Record: tool call header format, progress indicators, result truncation,
   expandable/collapsible sections.

F) SCROLLING & VIEWPORT
   - Scroll up through history (mouse wheel, page up, arrow keys)
   - Scroll down back to bottom
   - Auto-scroll behavior when new content arrives
   - Scroll while agent is streaming

G) RESIZE BEHAVIOR
   tuistory -s ref resize 60 20
   tuistory -s ref snapshot --trim
   tuistory -s ref resize 200 50
   tuistory -s ref snapshot --trim
   tuistory -s ref resize 120 40
   tuistory -s ref snapshot --trim
   Record: reflow behavior, truncation, responsive layout changes.

H) ERROR STATES
   - Disconnect network and try a message
   - Try a command that doesn't exist
   - Trigger a tool error (e.g., read nonexistent file)
   Record: error message format, recovery behavior.

I) CONVERSATION MANAGEMENT
   - Multiple back-and-forth messages
   - How conversation history appears
   - Visual separation between user/assistant messages
   - Timestamps or metadata shown

J) STATUS INDICATORS
   - Spinner/progress during agent thinking
   - Token count or cost display
   - Model name display
   - Connection status
   - Any status bar contents

OUTPUT FORMAT:

Produce a file called features.jsonl where each line is:

{"id": "A1", "category": "input", "name": "basic message submit", "interaction": "typed 'hello' and pressed enter", "behavior": "message appears in chat with '>' prefix, agent responds with streaming text", "snapshot_key": "after-basic-submit"}

Also produce features-summary.md with a human-readable table:

| ID | Category | Feature | Behavior | Priority |
|----|----------|---------|----------|----------|
| A1 | input | basic submit | works | high |

Priority should be: high (core UX), medium (polish), low (nice-to-have).

4. Close the session when done:
   tuistory -s ref close

RULES:
- Take a snapshot after EVERY interaction. Do not guess behavior.
- If the app shows a dialog or prompt you didn't expect, document it and
  navigate through it before continuing.
- Be exhaustive. A missing feature in the catalog means it won't get
  implemented in golem.
- If the app crashes or hangs, document it and restart.
```

---

## Prompt 2: Per-Feature Comparison & Fix (run once per feature)

```
You are a TUI engineer improving golem to match a reference TUI's behavior.

TASK: Compare and fix a single feature.

FEATURE TO COMPARE:
{{FEATURE_ID}}: {{FEATURE_DESCRIPTION}}
Reference behavior: {{REFERENCE_BEHAVIOR}}

PROJECT: ~/ws/golem
LANGUAGE: Go (Bubble Tea v2 + Lip Gloss v2)
BUILD: cd ~/ws/golem && go build -o /tmp/golem .
TEST: cd ~/ws/golem && go test ./...

SOURCE STRUCTURE:
  main.go                    -- entry point, CLI arg parsing
  internal/ui/app.go         -- main Bubble Tea model (Model struct, Update, View)
  internal/ui/commands.go    -- tea.Cmd definitions
  internal/ui/styles/        -- lipgloss style definitions
  internal/ui/chat/          -- chat message rendering (markdown, tool results)
  internal/ui/plan/          -- plan display panel
  internal/ui/verification/  -- verification display
  internal/ui/invariants/    -- invariant checking
  internal/ui/common/        -- shared utilities (markdown, syntax highlighting)
  internal/agent/            -- agent runtime, tool orchestration
  internal/config/           -- configuration loading
  internal/eval/             -- evaluation logic

TOOLS: You have tuistory installed. Use it for all TUI interaction.

WORKFLOW:

Step 1: Understand the reference behavior
  Read the feature description carefully. If unclear, launch the reference TUI
  to observe it directly:
    tuistory launch "droid" -s ref --cols 120 --rows 40
    <perform the interaction described in the feature>
    tuistory -s ref snapshot --trim
    tuistory -s ref close

Step 2: Test golem's current behavior
    cd ~/ws/golem && go build -o /tmp/golem .
    tuistory launch "/tmp/golem" -s golem --cols 120 --rows 40
    <perform the same interaction>
    tuistory -s golem snapshot --trim
    tuistory -s golem close

Step 3: Compare
  Diff the two behaviors. Classify the result:
  - MATCH: golem already handles this correctly. Report and stop.
  - PARTIAL: golem handles it but with differences. List the gaps.
  - MISSING: golem doesn't handle this at all.
  - BETTER: golem actually does this better. Report and stop.

Step 4: Read the relevant source
  Based on the feature category, read the relevant Go source files.
  Map the feature to specific functions and types. Understand the current
  implementation before changing anything.
  Key files to check by category:
    - Input behavior → app.go (Update method, handleKeyPress)
    - Keyboard shortcuts → app.go (key handling in Update)
    - Slash commands → app.go (handleSlashCommand or similar)
    - Response rendering → chat/messages.go, common/markdown.go
    - Tool use display → chat/write_result.go, app.go (toolCallMsg/toolResultMsg)
    - Scrolling → app.go (viewport handling)
    - Resize → app.go (WindowSizeMsg handling)
    - Styles → styles/styles.go
    - Status indicators → app.go (View method, spinner)

Step 5: Implement the fix
  Make targeted, minimal changes. Follow these rules:
  - Match existing code style exactly (look at surrounding code)
  - Do not refactor unrelated code
  - Do not add dependencies unless absolutely necessary
  - If adding a new keybinding, follow the existing pattern in Update()
  - If adding a new style, define it in styles/styles.go
  - If adding a new component, follow the Bubble Tea Init/Update/View pattern
  - Preserve all existing tests. Do not break them.

Step 6: Build and test
    cd ~/ws/golem && go build -o /tmp/golem . && go test ./...
  If tests fail, fix them before proceeding.

Step 7: Verify with tuistory
    tuistory launch "/tmp/golem" -s golem --cols 120 --rows 40
    <perform the interaction again>
    tuistory -s golem snapshot --trim
    tuistory -s golem close
  Compare the new snapshot to the reference behavior.
  If it doesn't match, go back to Step 5.

Step 8: Report
  Output a structured result:

  FEATURE: {{FEATURE_ID}}
  STATUS: FIXED | PARTIAL_FIX | ALREADY_MATCHED | CANNOT_FIX | BETTER_IN_GOLEM
  CHANGES:
    - file: internal/ui/app.go
      description: Added ctrl+l keybinding to clear chat history
    - file: internal/ui/styles/styles.go
      description: Added ClearConfirmStyle for the confirmation dialog
  BEFORE_SNAPSHOT: <paste trimmed snapshot before fix>
  AFTER_SNAPSHOT: <paste trimmed snapshot after fix>
  NOTES: <any caveats, things that couldn't be matched exactly, or
          suggestions for follow-up>

RULES:
- ONE feature per invocation. Do not scope-creep.
- Always build and test before declaring done.
- Always verify visually with tuistory before declaring done.
- If the fix requires architectural changes beyond the scope of one feature,
  report CANNOT_FIX with a description of what's needed.
- If you break existing tests and can't fix them within 3 attempts, revert
  all changes and report CANNOT_FIX.
- Do not modify test files to make them pass. Fix the implementation.
```

---

## Orchestration

Run Prompt 1 once to produce `features.jsonl`. Then iterate over each feature:

```bash
# Pseudocode for orchestration
for feature in $(cat features.jsonl | jq -c '.'); do
  FEATURE_ID=$(echo $feature | jq -r '.id')
  FEATURE_DESC=$(echo $feature | jq -r '.name')
  REFERENCE_BEHAVIOR=$(echo $feature | jq -r '.behavior')

  # Invoke the comparison agent (droid, claude, or any agent with tuistory)
  # substituting {{FEATURE_ID}}, {{FEATURE_DESCRIPTION}}, {{REFERENCE_BEHAVIOR}}
  # in Prompt 2
done
```

Or in Droid/Factory, use the Task tool to spawn a worker per feature:

```
Task(subagent_type="worker", prompt=<Prompt 2 with substitutions>)
```

You can run 3-5 features in parallel since each uses its own tuistory session name.

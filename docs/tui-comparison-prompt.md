# TUI Comparison & Improvement Prompt

Two prompts below. **Prompt 1** runs once to build a feature catalog from the reference TUI. **Prompt 2** runs per-feature to compare and fix golem.

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

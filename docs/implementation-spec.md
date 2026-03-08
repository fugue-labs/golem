# Golem Implementation Specification

## 1. Purpose

This document specifies the implementation plan for Golem: a terminal-first coding agent built in Go with a Bubble Tea UI, Gollem model/runtime integration, durable tool-driven workflows, and explicit completion proofing through plans and invariants.

The immediate goal is not to create a generic chat app. The goal is to ship a coding agent that is reliably useful on real software tasks: inspect a repository, decide what to change, make edits, run verification, recover from errors, and clearly communicate completion state.

## 2. Product Goals

### 2.1 Primary goals

1. **Reliable coding execution**
   - The agent must complete real repository tasks end-to-end, not just offer suggestions.
   - It must prefer concrete action over passive explanation.
2. **Strong execution discipline**
   - Read before edit.
   - Plan before non-trivial changes.
   - Verify with build/test commands before claiming completion.
3. **Auditable progress**
   - Plans and invariants must be visible in the UI and preserved across a run.
   - Users should be able to understand what the agent is doing and why it considers work done.
4. **Low-latency interactive UX**
   - Streaming text, thinking, tool calls, and tool results must be visible live.
   - The UI must remain responsive while work is in progress.
5. **Provider/runtime flexibility**
   - The agent must support multiple model providers and runtime options through config.
6. **Context resilience**
   - The agent must maintain coherent execution over multi-turn coding sessions with persistent tool state and bounded history.

### 2.2 Non-goals

1. Building a general-purpose autonomous background service in v1.
2. Supporting arbitrary desktop automation.
3. Optimizing for personality over reliability.
4. Shipping hidden, non-auditable “magic” behaviors that bypass visible tool state.

## 3. Target Users

1. **Primary**: developers working in a local repository who want a coding agent in the terminal.
2. **Secondary**: agent builders evaluating prompt/tool/runtime quality.
3. **Internal**: maintainers iterating on routing, prompts, evaluation, and UI quality.

## 4. Current Codebase Baseline

The current repository already contains a strong foundation:

- `internal/agent`
  - agent construction, model/provider wiring, runtime prompt construction, runtime-state preparation.
- `internal/agent/tools`
  - local file/system tools: bash, view, edit, write, glob, grep, ls.
- `internal/ui`
  - Bubble Tea app, streaming event handling, history, cancellation, runtime preparation.
- `internal/ui/plan`
  - plan normalization and progress accounting.
- `internal/ui/invariants`
  - invariant normalization and pass/fail accounting.
- `internal/config`
  - runtime/provider/config loading.
- `internal/skills`
  - optional skill loading.

The baseline architecture is good enough to evolve rather than rewrite.

## 5. Required Product Capabilities

### 5.1 Core task loop

For repository work, the agent must consistently follow this loop:

1. Inspect relevant files.
2. Form a small executable plan.
3. Perform edits or create new files.
4. Run verification commands.
5. Fix discovered issues.
6. Mark completion only when verification passes and hard invariants are satisfied.

### 5.2 Interactive streaming

The UI must stream:

- assistant text deltas
- thinking deltas
- tool calls
- tool results
- errors
- final completion state

### 5.3 Workflow proofing

For non-trivial work the runtime must support:

- a visible task plan
- invariant extraction
- invariant updates with evidence
- final invariant summary as a completion gate

### 5.4 Durable execution state

Each run must preserve:

- model conversation history
- tool session state
- plan state
- invariant state
- pending user messages queued while the agent is busy

### 5.5 Runtime adaptability

The system must support:

- provider selection
- model selection
- runtime prompt augmentation
- router-assisted team mode decisions
- optional code mode
- optional top-level personality
- bounded auto-context

## 6. Functional Requirements

### 6.1 Startup and configuration

The application must:

1. Start from `main.go`.
2. Load config from environment/flags/config file via `internal/config`.
3. Validate required provider credentials and runtime settings.
4. Surface effective runtime values in a machine-readable and human-readable form.

### 6.2 Runtime preparation

Before a run starts, the system must:

1. Compute a `RuntimeState` from config and prompt.
2. Resolve effective team mode.
3. Resolve router fallback behavior without crashing the run.
4. Decide code mode availability and expose status.

### 6.3 Agent construction

Agent construction must:

1. Create the provider-specific model.
2. Reuse or initialize a persistent `codetool.Session`.
3. Apply codetool options consistently.
4. Compose the base system prompt and dynamic runtime prompt.
5. Optionally add top-level personality.
6. Apply reasoning/thinking controls by provider.

### 6.4 Tool execution

The runtime must expose a tool surface sufficient for coding work, including at minimum:

- shell execution
- file viewing
- exact file editing
- file creation
- file discovery
- file-content search
- directory listing

The runtime should also support or verify availability for:

- multi-file editing
- background process management
- semantic code intelligence (LSP)
- subtask delegation
- structured plan state
- structured invariant state
- visual artifact inspection
- batched code-mode execution

If some of these are provided by Gollem/codetool rather than local package code, that availability must still be reflected accurately in runtime prompts, UI, and evaluation coverage.

### 6.5 UI behavior

The TUI must:

1. Accept user input while the agent is busy.
2. Queue pending user messages safely.
3. Render streaming content without corrupting message order.
4. Distinguish assistant output, thinking, tool calls, tool results, and errors.
5. Render plan and invariant summaries in a compact but legible format.
6. Support cancellation.
7. Support clear/reset behavior that also cleans session state.

### 6.6 Verification discipline

Before completion, the system must make it easy for the agent to:

1. rerun failing commands
2. inspect exact errors
3. update plan status
4. update invariant evidence
5. confirm final build/test results

## 7. Non-Functional Requirements

### 7.1 Reliability

- No silent tool failures.
- No hidden mutation of files outside declared tool operations.
- Runtime failures must be surfaced in the transcript.

### 7.2 Safety

- Tool descriptions and system prompt must enforce read-before-edit behavior.
- Dangerous commands are allowed only through explicit shell execution initiated by the model/user.
- The system must never claim verification that was not actually run.

### 7.3 Performance

- UI should remain interactive during long model/tool execution.
- Streaming updates should feel incremental, not buffered until completion.
- Context management should prevent runaway history growth.

### 7.4 Extensibility

- New tools, skills, routing policies, and render panels should be addable without rewriting the runtime.
- Internal state types should remain serializable and testable.

## 8. Architecture

### 8.1 High-level architecture

```text
main.go
  -> internal/config
  -> internal/ui
       -> internal/agent
            -> Gollem model + codetool runtime
            -> local tools / codetool tools
       -> chat / plan / invariants / styles
```

### 8.2 Package responsibilities

#### `internal/config`
- Parse and normalize runtime settings.
- Own defaults and validation.
- Present effective config to the rest of the app.

#### `internal/agent`
- Build models and runtime prompts.
- Prepare `RuntimeState`.
- Choose routing/team-mode behavior.
- Instantiate the coding agent with the correct tool/runtime stack.

#### `internal/agent/tools`
- Own local tool wrappers where custom behavior is required.
- Keep tool schemas, descriptions, and execution semantics precise.

#### `internal/ui`
- Own end-user interaction.
- Manage Bubble Tea model state, streaming events, cancellation, and rendering.
- Mirror plan/invariant/tool state from runtime messages into display state.

#### `internal/ui/chat`
- Structure chat/tool/thinking messages.
- Own formatting and render-safe message segmentation.

#### `internal/ui/plan`
- Normalize external plan state for display.
- Provide progress math and display helpers.

#### `internal/ui/invariants`
- Normalize invariant state for display.
- Provide hard/soft pass/fail accounting.

#### `internal/skills`
- Load reusable optional instruction bundles.

## 9. Data Model Specification

### 9.1 Runtime state

Current baseline:

```go
type RuntimeState struct {
    EffectiveTeamMode bool
    TeamModeReason    string
    CodeModeStatus    string
    CodeModeError     string
    Session           *codetool.Session
}
```

Target additions (if not already available upstream):

```go
type RuntimeDiagnostics struct {
    RouterModel       string
    ProviderTransport string
    AutoContextActive bool
    SkillNames        []string
}
```

This should remain derivable from config and runtime decisions rather than becoming a mutable dumping ground.

### 9.2 UI run state

The UI model should continue owning:

- current run ID
- busy/cancel state
- history
- message list
- usage
- plan state
- invariant state
- pending queued user messages

Optional future additions:

- run start/end timestamps per turn
- latest verification summary
- last successful command
- panel visibility toggles

### 9.3 Plan state

The plan state must support:

- ordered task list
- normalized status values (`pending`, `in_progress`, `completed`, `blocked`)
- progress summary
- optional notes per task

### 9.4 Invariant state

The invariant state must support:

- ordered items
- normalized kind (`hard`, `soft`)
- normalized status (`unknown`, `in_progress`, `pass`, `fail`)
- evidence text
- extracted/not-extracted state

## 10. End-to-End Execution Flow

### 10.1 User submits a prompt

1. UI records the prompt.
2. UI allocates a new `runID`.
3. UI starts runtime preparation asynchronously.
4. UI enters busy state and keeps input enabled.

### 10.2 Runtime preparation phase

1. `agent.PrepareRuntime` evaluates routing/team mode and code mode availability.
2. UI receives `runtimePreparedMsg`.
3. UI creates or refreshes the agent with the prepared runtime.
4. UI starts the model run with hooks for streaming/tool events.

### 10.3 Streaming execution phase

During execution the model may emit:

- text deltas
- thinking deltas
- tool calls
- tool results

The UI must ignore stale messages whose `runID` no longer matches the active run.

### 10.4 Tool-state mirroring

After each tool result and at run completion:

1. Pull plan state from tool/runtime state.
2. Pull invariant state from tool/runtime state.
3. Normalize for display.
4. Re-render summaries.

### 10.5 Completion phase

On success:

1. Persist updated history.
2. Capture final usage.
3. Update plan/invariant panes.
4. Return focus to input.

On failure:

1. Append visible error message.
2. Preserve partial transcript and tool history.
3. Return focus to input.

## 11. System Prompt and Behavioral Contract

The system prompt is a product surface, not a random instruction blob. It must define:

1. Mission: best-in-class terminal coding agent.
2. Critical rules: read-before-edit, act early, verify changes, no TODOs, no unsupported claims.
3. Communication style: concise but not cryptic.
4. Workflow: search/read/plan/act/test/verify.
5. Tool-usage rules: exact semantics for edit/write/bash/etc.
6. Completion rules: no completion without build/test verification and hard invariant resolution.

### 11.1 Prompt composition contract

The final runtime prompt must be composed from:

1. static system prompt from `internal/agent/system_prompt.md`
2. dynamic runtime profile (`buildRuntimePrompt`)
3. optional top-level personality
4. optional loaded skills

### 11.2 Prompt maintenance requirements

- Keep tool descriptions aligned with actual runtime capabilities.
- Avoid duplicated instructions across static and dynamic prompts.
- Test for prompt regressions that reintroduce contradictory guidance.

## 12. Tooling Specification

### 12.1 Local tools

Each local tool must define:

- stable schema
- clear description of intended use
- deterministic output format
- safe path resolution relative to working directory
- actionable errors

### 12.2 Editing guarantees

`edit` must continue to enforce exact-match replacement.

Requirements:

1. Fail when `old_string` is not found.
2. Fail when ambiguous unless `replace_all` is explicitly requested.
3. Preserve unrelated file content byte-for-byte.

### 12.3 View guarantees

`view` must:

- support offsets and limits
- preserve line numbers
- support negative offsets from file end
- produce output suitable for precise edit preparation

### 12.4 Shell guarantees

`bash` must:

- execute in the configured working directory
- return exit code and output
- support foreground and background modes if available
- make failure unmistakable in output formatting

### 12.5 Higher-level tool expectations

Whether implemented locally or provided by Gollem, the runtime should expose and test:

- `planning`
- `invariants`
- `delegate`
- `execute_code`
- `lsp`
- `open_image`
- `multi_edit`
- background process status/kill

If any of these are unavailable in a given build/runtime, the runtime prompt and UI should state that clearly instead of pretending they exist.

## 13. UI Specification

### 13.1 Layout

The UI should present:

1. main transcript area
2. input area
3. status/runtime area
4. plan summary area
5. invariant summary area

The exact layout can remain compact, but plan/invariant information must be continuously accessible.

### 13.2 Message rendering

Message types:

- user
- assistant
- thinking
- tool call
- tool result
- error

Requirements:

- preserve order
- render tool status transitions (`running`, `done`, `error`)
- avoid duplicate assistant message fragments
- support markdown-like formatting where useful

### 13.3 Busy-state behavior

While busy:

- the user can continue typing
- additional user submissions are queued
- cancellation remains available
- spinner/status reflects in-progress work

### 13.4 Clear/reset behavior

`/clear` or equivalent must:

- clear transcript/history as intended
- reset plan state
- reset invariant state
- cleanup persistent session state where appropriate
- avoid leaking stale `runID` event handling into the next turn

## 14. Evaluation Framework

A serious coding agent needs a first-class eval suite.

### 14.1 Eval categories

1. **Prompt-only behavior**
   - casual chat responses stay brief
   - coding tasks trigger planning/invariants when appropriate
2. **Tool discipline**
   - reads before edits
   - uses exact-match edits
   - reruns failing commands after fixes
3. **Repository tasks**
   - bug fix
   - feature addition
   - refactor
   - test repair
   - docs/spec generation
4. **Failure recovery**
   - syntax error after edit
   - failing test after partial implementation
   - missing file/function
5. **UI-state correctness**
   - stale run events are ignored
   - cancellation works
   - plan/invariant panes update correctly

### 14.2 Eval harness outputs

Each eval should capture:

- prompt
- model/provider/runtime settings
- transcript
- tool call sequence
- final files changed
- verification commands run
- pass/fail result
- rubric notes

### 14.3 Success metrics

Core metrics:

- task success rate
- verified success rate
- false-completion rate
- average tool calls per successful task
- average turns to completion
- rate of read-before-edit violations
- hard-invariant unresolved rate

## 15. Testing Strategy

### 15.1 Unit tests

Continue and expand tests for:

- config parsing/validation
- runtime routing decisions
- prompt composition
- plan normalization
- invariant normalization
- message formatting
- command parsing
- tool wrappers

### 15.2 Integration tests

Add integration coverage for:

- full run creation with mocked model
- streaming event handling
- tool state propagation into UI state
- cancellation behavior
- session reuse/reset

### 15.3 Golden tests

Use golden tests for:

- runtime prompt output
- rendered message formatting
- plan/invariant summaries
- help/command output where stable

### 15.4 Manual verification

Every release candidate should be manually tested on at least:

1. one bug fix task
2. one feature implementation task
3. one repo-introspection/doc-writing task
4. one failure-recovery task

## 16. Observability and Diagnostics

The app should log or expose enough data to answer:

- what runtime was actually selected?
- why was team mode enabled or disabled?
- was code mode available?
- what tool sequence occurred?
- what verification command determined completion?
- why did a run fail?

Recommended diagnostics surfaces:

1. runtime profile in prompt/UI
2. optional debug log file
3. structured run summary for eval capture

## 17. Security and Safety Constraints

1. Respect working-directory boundaries.
2. Make file mutation explicit through tools.
3. Do not silently fetch remote content unless configured and surfaced.
4. Do not claim tests/builds passed without executing them.
5. Do not hide failed invariants.
6. Keep prompt instructions aligned with actual runtime power.

## 18. Rollout Plan

### Phase 1: Reliability baseline

Scope:

- tighten core agent/runtime prompt alignment
- ensure plan/invariant state always mirrors correctly in UI
- close clear/reset/session cleanup gaps
- improve test coverage for runtime and UI event flow

Acceptance:

- `go test ./...` passes
- runtime prompt tests cover effective runtime rendering
- UI tests cover plan/invariant updates and stale-run filtering

### Phase 2: Tooling completeness

Scope:

- verify and expose full tool surface
- add missing wrappers or runtime disclosures
- improve error formatting and long-running process handling

Acceptance:

- tool availability is accurately represented
- eval tasks no longer fail due to mismatched prompt/tool claims

### Phase 3: Evaluation and hardening

Scope:

- build repeatable eval harness
- add golden/integration cases for real repository tasks
- track reliability metrics across model/runtime variants

Acceptance:

- can compare model/runtime configs on the same task set
- false-completion rate is measurable and trending down

### Phase 4: UX polish

Scope:

- better panel layout
- richer summaries
- improved rendering for tool args/results and markdown
- optional run summaries/export

Acceptance:

- users can follow execution without opening logs

## 19. Concrete Implementation Backlog

### 19.1 Runtime / agent

- Audit `internal/agent/system_prompt.md` against actual tool/runtime capabilities.
- Add or strengthen tests around prompt composition and provider-specific options.
- Make runtime-profile reporting exhaustive but non-redundant.
- Ensure router fallback behavior is visible and testable.

### 19.2 UI

- Audit `internal/ui/app.go` for run lifecycle edge cases.
- Add tests for pending-message queue behavior.
- Add tests for cancel/clear/session cleanup.
- Ensure plan and invariant panes update both on tool result and final run completion.

### 19.3 Tools

- Audit local tool descriptions/output consistency.
- Add tests for exact-match edit behavior and path handling.
- Verify higher-level Gollem tool availability and disclose any unavailable tools.

### 19.4 Eval infrastructure

- Define eval case schema.
- Implement transcript and tool-trace capture.
- Add a small curated real-task benchmark set.

## 20. Acceptance Criteria

The implementation is complete when all of the following are true:

1. A developer can point Golem at a repository and successfully complete common coding tasks.
2. The agent visibly plans non-trivial work.
3. Hard invariants are tracked and used as completion gates.
4. The UI remains responsive during long runs.
5. Runtime/tool availability shown to the model and user matches reality.
6. Tests cover prompt composition, UI state flow, and core normalization logic.
7. A repeatable eval suite exists for regression detection.

## 21. Open Questions

1. Which higher-level tools are locally implemented vs inherited from Gollem, and how should that distinction appear in docs/UI?
2. Should run transcripts/tool traces be persisted to disk by default or only in debug/eval mode?
3. Should team-mode routing be surfaced as a visible per-run badge in the UI?
4. Should plan/invariant summaries become dedicated panes or remain inline/secondary panels?
5. What minimal eval set best predicts real-user satisfaction?

## 22. Recommended Next Steps

1. Audit prompt/tool/runtime parity first.
2. Add missing UI lifecycle tests second.
3. Stand up the eval harness third.
4. Iterate on UX only after reliability metrics improve.

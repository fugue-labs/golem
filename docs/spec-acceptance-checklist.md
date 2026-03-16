# Implementation Spec Acceptance Checklist

This checklist audits the current implementation against `docs/implementation-spec.md` §20 and adds the execution-ready non-regression baseline for shell-facing work.

## TUI baseline, rubric, and preserved-UX acceptance addendum

Use this addendum when evaluating TUI changes so reviewers can verify three things from one place:

1. the target shipped surface,
2. the preserved e2e contract that must not regress, and
3. the implementation order that matches the current code layout.

### Required shipped-surface coverage

Documentation and implementation tasks should explicitly score or discuss these five shipped surfaces:

1. **Shell**
   - Entry point: `main.go`
   - Primary implementation: `internal/ui/app.go`, `internal/ui/styles/styles.go`
   - Review focus: launch frame, `GOLEM` identity, input visibility, status bar, minimum-size behavior, shell responsiveness

2. **Transcript**
   - Primary implementation: `internal/ui/app.go`, `internal/ui/chat/messages.go`
   - Review focus: user/assistant/tool/system/error rendering, scrolling, slash-command output, busy-state readability

3. **Workflow rail**
   - Primary implementation: `internal/ui/workflow_panel.go`
   - Supporting state: `internal/ui/app.go`
   - Review focus: active work, blockers, approvals, verification, invariants, and next-action visibility

4. **Dashboard**
   - Entry point: `main.go` `dashboard` subcommand
   - Primary implementation: `internal/ui/dashboard/dashboard.go`, `internal/ui/styles/styles.go`
   - Review focus: Mission Control identity, pane scanability, empty/error state quality, pane navigation

5. **Discoverability**
   - Primary implementation: `internal/ui/app.go`, `internal/ui/commands.go`, `internal/ui/dashboard/dashboard.go`
   - Review focus: `/help`, `/search <query>` usage, tab completion, launch guidance, placeholder copy, key hints

### Preserved e2e UX contracts

Shell-facing changes should be rejected unless they preserve these verified behaviors or intentionally update tests and docs together:

- visible `GOLEM` at launch plus a visible prompt or `Ask anything… /help for commands`,
- `/help` discoverability for key commands and key hints,
- `/search <query>` usage text including `search across all saved sessions` and `Examples`,
- `golem dashboard` launch stability into `Mission Control` or a valid empty/error state,
- dashboard pane navigation with `Tab`, `Shift+Tab`, `1-4`, and `j/k`,
- stable cancellation behavior via `Esc`,
- stable transcript paging via `PgUp/PgDn`,
- stable input-history behavior via `↑/↓`, and
- stable `/clear`, `/model`, `/doctor`, `/cost`, replay/rewind, unknown-command, tab-completion, and slash-command sequencing behavior.

### Required implementation order alignment

Reviewers should expect TUI work to follow the current architecture instead of an abstract product order.

1. **Shell entry and framing first**
   - `main.go`
   - `internal/ui/app.go`
   - `internal/ui/styles/styles.go`
2. **Discoverability and command contract second**
   - `internal/ui/commands.go`
   - slash-command dispatch in `internal/ui/app.go`
3. **Transcript readability third**
   - `internal/ui/chat/messages.go`
   - transcript composition and scroll behavior in `internal/ui/app.go`
4. **Workflow rail fourth**
   - `internal/ui/workflow_panel.go`
   - width-gating and integration in `internal/ui/app.go`
5. **Dashboard fifth**
   - `main.go` dashboard routing
   - `internal/ui/dashboard/dashboard.go`
   - cross-surface style alignment in `internal/ui/styles/styles.go`

### Review checklist for any TUI task

A TUI change should name:

1. the target surface: shell, transcript, workflow rail, dashboard, or discoverability,
2. the exact preserved e2e contract items it cannot regress,
3. the primary implementation files touched in `main.go` or `internal/ui`, and
4. the execution phase above that justifies the order of work.

## Mission orchestration acceptance addendum

Use this addendum when the work touches mission lifecycle, approvals, dashboard copy, or Mission Control behavior.

### Required mission-behavior alignment

Documentation and implementation should agree on the currently shipped mission contract:

1. **Lifecycle**
   - Mission statuses include `draft`, `planning`, `awaiting_approval`, `running`, `blocked`, `paused`, `completing`, `completed`, `failed`, and `cancelled`.
   - `/mission new <goal>` creates a durable draft mission using repo metadata supplied by the TUI.
   - Reviewers should document current mission-creation metadata precisely: the TUI supplies `repo_root` and `base_branch`, while `base_commit` is not yet populated because `gitCommit()` still returns an empty string.
   - Reviewers must not claim repository precondition enforcement inside `CreateMission` unless that validation code actually ships.
   - Operator guidance must distinguish `awaiting_approval` from the summary phase label **`Ready to start`**.

2. **Approval/start semantics**
   - `/mission plan` creates the durable mission-plan approval gate.
   - `/mission approve` resolves that gate durably through `ApproveMission` and immediately attempts start; if another approval still blocks execution, the operator should see approved-but-not-started guidance instead of a silent failure.
   - `/mission start` cannot bypass a pending plan approval.
   - `/mission start` may begin execution from `awaiting_approval` only after the plan gate is approved and no other approvals remain pending.
   - Resume semantics are `/mission start` from `paused`.
   - Reviewers should treat approval and start as separate operator-visible steps even when `/mission approve` triggers both in sequence.

3. **Orchestration responsibilities**
   - The controller owns lifecycle transitions and mission summaries.
   - The scheduler/worker launcher dispatches ready work safely.
   - The orchestrator runs an in-process tick loop that dispatches workers, dispatches reviewers, integrates accepted work, checks completion, and emits transient operator-facing event-bus updates.
   - The shipped TUI recreates orchestration in-process on `/mission start`; reviewers should not describe a daemon-backed resume/attach flow unless that implementation ships.

4. **Persistence and dashboard behavior**
   - Mission Control and `/mission status` must read durable mission state rather than rely on transcript-only memory.
   - `golem dashboard` must preserve its Mission Control empty state and its four-pane model: Tasks, Workers, Evidence, Events.
   - Dashboard headers and evidence/examples should align with current rendering: status/task progress/worker/approval metrics in the header, pending approvals in Evidence, and recent durable events in Events.
   - When no mission ID is supplied, the dashboard should prefer the highest-priority active mission (`running` > `blocked` > `paused` > `awaiting_approval` > `planning` > `draft`).
   - Reviewers should accept the current shipped empty-state line `Create one with /mission new or run golem mission new.` as UI copy, but must document the `golem mission new` phrase as aspirational rather than a shipped command family.
   - Event examples must distinguish persisted mission-store names (for example `mission.created`, `plan.applied`, `review.passed`, `integration.conflict.requeued`) from transient orchestrator/TUI event-bus names (for example `worker.started`, `review.pass`, `review.request_changes`).
   - Documentation should keep unsupported reject/retry/replan/escalation actions clearly marked aspirational rather than describing them as shipped operator controls.

## Acceptance Criteria

1. **Partial** — A developer can point Golem at a repository and complete common coding tasks.
   - Core runtime, tooling, workflow tracking, and verification flows are implemented.
   - The new eval fixtures cover representative prompt-only, repo-introspection, verification-required, and failure-recovery cases.
   - This remains marked partial because the benchmark set is intentionally small and not yet scored by an automated evaluator.

2. **Pass** — The agent visibly plans non-trivial work.
   - Planning state is mirrored into the UI and exposed through workflow summaries.

3. **Pass** — Hard invariants are tracked and used as completion gates.
   - Invariant extraction/normalization remains wired into runtime and UI state.

4. **Pass** — The UI remains responsive during long runs.
   - Busy-state queueing, cancellation, stale-event filtering, and workflow persistence are covered by tests.

5. **Pass** — Runtime/tool availability shown to the model and user matches reality.
   - Runtime validation, shared runtime reporting, machine-readable status/runtime output, and prompt/UI parity now share one report model.

6. **Pass** — Tests cover prompt composition, UI state flow, and core normalization logic.
   - Added prompt/runtime golden tests, lifecycle regression tests, config validation tests, and eval schema tests.

7. **Pass** — A repeatable eval suite exists for regression detection.
   - The repo now contains eval case schema, curated benchmark fixtures, structured run summaries, and golden output tests.

## Roadmap Boundary

The persistent-loop / daemon design in `docs/persistent-loop-design.md` remains out of scope for this acceptance audit.

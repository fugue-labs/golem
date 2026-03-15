# Implementation Spec Acceptance Checklist

This checklist audits the current implementation against `docs/implementation-spec.md` §20.

## TUI rubric and preserved-UX acceptance addendum

Use this addendum when evaluating shell-facing work so reviewers can verify both UX ambition and regression safety from one place.

### Required rubric coverage

Documentation and implementation tasks should explicitly score or discuss these five surfaces:

1. **Launch frame** — first-frame clarity, visible identity, and immediate input orientation.
2. **Transcript readability** — separation between user, assistant, tool, system, and error states.
3. **Workflow visibility** — ability to identify active work, blockers, next action, and proof state quickly.
4. **Dashboard readability** — Mission Control scanability before interaction and during pane navigation.
5. **Discoverability** — persistence of help, slash-command cues, usage text, and key hints.

### Preserved e2e UX contracts

Shell-facing changes should be rejected unless they preserve these verified behaviors or intentionally update tests and docs together:

- visible `GOLEM` at launch plus a visible prompt or `Ask anything… /help for commands`,
- `/help` discoverability for key commands and key hints,
- `/search <query>` usage text including `search across all saved sessions` and `Examples`,
- `golem dashboard` launch stability into Mission Control or a valid empty/error state,
- stable cancellation, scroll, and input-history behavior (`Esc`, `PgUp/PgDn`, `↑/↓`), and
- stable `/clear`, `/model`, `/doctor`, unknown-command, and tab-completion behavior.

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
   - `/mission approve` resolves that gate durably through `ApproveMission` and immediately attempts start.
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

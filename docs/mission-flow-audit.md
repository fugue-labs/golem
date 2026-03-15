# Mission flow audit

Date: 2026-03-15

## Scope

Audited these mission code paths and surfaces before implementation work:

- `internal/mission/store.go`
- `internal/mission/types.go`
- `internal/mission/controller.go`
- `internal/mission/summary.go`
- `internal/mission/scheduler.go`
- `internal/mission/orchestrator.go`
- `internal/mission/memory_store.go`
- `internal/mission/sqlite_store.go`
- `internal/mission/worker.go`
- `internal/mission/reviewer.go`
- `internal/mission/integrator.go`
- `internal/ui/mission_commands.go`
- `internal/ui/mission_planning.go`
- `internal/ui/mission_panel.go`
- `internal/ui/dashboard/dashboard.go`
- `internal/ui/mission_planning_test.go`
- `internal/ui/mission_panel_test.go`
- `test/e2e/tuistory_test.go`
- `docs/mission-orchestration-prd.md`

## Baseline verification

- `go test ./...` ✅
- Current codebase is green, so the audit focuses on **implementation gaps, state-machine holes, and weakly-covered edges**, not current red tests.

## End-to-end mission flow map

### 1. Mission creation and persistence

1. TUI `/mission new <goal>` calls `Model.handleMissionNew`.
2. `handleMissionNew` builds a `mission.CreateMissionRequest` from the current repo metadata.
3. `Controller.CreateMission` creates a draft mission and appends `mission.created`.
4. Persistence is provided by the `Store` interface, with concrete implementations in SQLite and memory stores.
5. The normal TUI lazily opens SQLite and silently falls back to in-memory storage if SQLite open fails; the dashboard opens SQLite only.

### 2. Planning and plan application

1. `/mission plan` marks the mission `planning` in `handleMissionPlan`.
2. The planner run returns JSON parsed by `mission.ParsePlanResult`.
3. `Controller.ApplyPlan` validates the plan, creates tasks, adds dependencies, resolves dependency-free tasks to `ready`, then moves the mission to `awaiting_approval`.
4. `completeMissionPlanRun` restores the previous mission status on parse/apply failure.

### 3. Approval and mission start

1. `/mission approve` currently just calls `Controller.StartMission`.
2. `StartMission` allows `awaiting_approval` and `paused` missions to transition to `running`.
3. The TUI then starts `mission.Orchestrator`.

### 4. Scheduling and worker dispatch

1. Orchestrator tick order is: dispatch workers → dispatch reviewers → integrate accepted work → completion check.
2. `Scheduler.SelectTasks` reads mission budget, gets ready tasks, counts active runs, applies concurrency, then filters for write-scope conflicts.
3. `WorkerLauncher.DispatchReadyTasks` provisions worktrees, marks tasks `running`, creates worker runs, and emits `worker.dispatched`.
4. Worker completion moves tasks to `awaiting_review`; worker failure requeues or permanently fails tasks based on attempt count.

### 5. Review and acceptance

1. `ReviewLauncher.DispatchPendingReviews` finds tasks in `awaiting_review` and creates review runs.
2. Reviewer output is parsed by `ParseReviewResult`.
3. Review pass moves the task to `accepted`.
4. Reject/request-changes requeue the task to `ready` with feedback stored in `BlockingReason`.
5. Review failures keep the task in `awaiting_review` unless repeated failures trigger auto-accept.

### 6. Integration and completion

1. `IntegrationEngine.IntegrateReady` looks for `accepted` tasks.
2. `IntegrateTask` checks dependencies, merges the worker branch into the mission base branch, and moves the task to `integrated`.
3. Merge conflicts requeue the task to `ready` with conflict guidance.
4. `CheckMissionComplete` ends the mission when all tasks are `integrated` or `done`.

### 7. Status summaries and UI rendering

1. `Controller.GetMissionSummary` uses `BuildMissionSummary`, not the store aggregate summary helpers.
2. Summary output drives:
   - `/mission status`
   - mission workflow panel rendering
   - dashboard mission header and task panes
3. E2E coverage confirms core mission commands and dashboard startup/navigation are working from a user perspective.

## Concrete mission invariants

These are the invariants the current implementation is trying to preserve and that follow from the PRD.

### Mission lifecycle invariants

1. Mission truth is durable in the store, not only in transcript text.
2. A mission starts in `draft` and only becomes `running` after planning and approval.
3. Terminal mission states are `completed`, `failed`, and `cancelled`.
4. A mission should not claim to be `running` when there is no safe forward progress path.
5. Pause/cancel should leave persistent run/task state recoverable and internally consistent.

### DAG and task invariants

6. Tasks belong to exactly one mission.
7. Dependencies must only relate tasks within the same mission DAG.
8. Tasks with all dependencies satisfied become `ready`; unresolved dependencies keep tasks non-ready.
9. Accepted work should not integrate before dependency prerequisites are integrated/done.
10. Retry state must preserve enough feedback for the next attempt.

### Concurrency and isolation invariants

11. Parallel work is allowed only when writable scopes do not overlap.
12. Active work must exclude conflicting ready tasks, including against already-running work.
13. Worker leases and heartbeats should be recoverable after cancellation or process loss.

### Review and integration invariants

14. Review is independent of worker claims.
15. Reviewed work should not affect the integration branch before passing review.
16. Approval gates should be durably represented when policy requires them.
17. Integration should be deterministic, dependency-aware, and evidence-producing.

### Summary / TUI invariants

18. Mission summaries should match durable store state, not ephemeral UI state.
19. Dashboard and workflow panel should show the same lifecycle truth.
20. Approval, blockers, ready work, active work, and recent events should all be operator-visible.

## Confirmed gaps and weak edges

### A. Active-run cancellation does not persist state cleanly

**Area:** orchestrator pause/cancel/stop behavior  
**Severity:** high

`Model.handleMissionPause` and `handleMissionCancel` stop the orchestrator before updating mission status. `Orchestrator.Stop` cancels agent contexts, but `runWorker` and `runReviewer` return early on context cancellation without calling `CancelWorker` or any equivalent persistent-state transition.

Resulting risk:

- runs can remain `running`
- tasks can remain `running` or `awaiting_review`
- pause/cancel may rely on later recovery, but recovery is not wired into the normal start path

This is the most important state-consistency edge to fix before more mission features are added.

### B. Scheduler conflict detection is ineffective against already-running tasks in production

**Area:** scheduler / worker isolation  
**Severity:** high

`Scheduler.SelectTasks` compares candidate `Task.Scope.WritePaths` against active run `WorktreePath` values. In production, `WorktreePath` is a filesystem path like a worktree directory, not the running task's writable scope.

That means:

- intra-batch scope conflicts are filtered correctly
- conflicts against already-running tasks are **not** reliably filtered in real runs

So the PRD invariant “parallelize only where ownership is clear” is only partially enforced.

### C. Plan approval is not durably represented as an approval record

**Area:** controller / approvals / dashboard evidence  
**Severity:** medium-high

`Controller.ApplyPlan` moves the mission to `awaiting_approval`, but it does not create a pending `Approval` row.

Implications:

- the mission lifecycle has an approval gate, but the approval store does not record it
- dashboard evidence pane only shows pending approval rows, so mission-start approval is underrepresented there
- this weakens the PRD requirement that approvals be explicit and durable

### D. Plan application is non-transactional and can partially apply

**Area:** controller / store consistency  
**Severity:** high

`Controller.ApplyPlan` creates tasks, then dependencies, then resolves ready tasks, then updates the mission status. If any step fails partway through, previously-created tasks/dependencies remain in the store.

UI code restores the mission status on planning failure, but it cannot roll back partially-written store state.

This is especially risky for duplicate task IDs or partial persistence failures.

### E. Recovery exists, but the normal TUI/orchestrator path does not wire it in

**Area:** restart / stale lease recovery  
**Severity:** high

There is substantial recovery logic elsewhere in `internal/mission`, but `startOrchestrator` does not invoke it, and the orchestrator tick shown here does not reconcile stale runs before dispatch.

So mission restart/re-attach behavior remains weaker than the PRD expects unless another entrypoint invokes recovery first.

### F. Review auto-accept after repeated reviewer failures weakens the “review required” guarantee

**Area:** review policy  
**Severity:** medium-high

`ReviewLauncher.FailReview` auto-accepts a task after `maxReviewFailures` consecutive review failures.

That may be pragmatic for infrastructure issues, but it is a policy deviation from the PRD’s strong statement that review is mandatory before integration.

This should be treated as an explicit policy exception, not silent default behavior.

### G. Mission-level blocked / failed / completing phases are only partially driven

**Area:** lifecycle state machine  
**Severity:** medium

Mission statuses such as `blocked`, `failed`, and `completing` exist in types and UI rendering, but the main controller/orchestrator flow shown here does not actively drive most of them.

Observed effect:

- a mission can remain `running` while all actionable progress is blocked
- task-level blockers surface in summaries, but mission lifecycle may not advance to `blocked`

The UI currently papers over this by surfacing blocked tasks and attention text.

### H. Store aggregate summaries diverge from the canonical summary builder

**Area:** store / summary  
**Severity:** medium

`Controller.GetMissionSummary` uses `BuildMissionSummary`, but store implementations also expose `GetMissionSummary`. The store versions only compute counts and active run totals; they do not compute dependency edges, focus task, next task, ready/review/blocked task lists, or derived display fields.

This creates two summary semantics:

- canonical rich summary via controller
- reduced store summary via store methods

That divergence is easy to misuse in future code.

### I. Dependency filtering is weak in in-memory helpers

**Area:** store / DAG integrity  
**Severity:** medium

`InMemoryStore.ListDependencies` includes a dependency when either endpoint belongs to the mission, instead of requiring both endpoints to belong to the same mission.

That means malformed cross-mission edges can leak into a mission’s dependency view.

SQLite summary helpers also duplicate DAG logic instead of centralizing it.

### J. Event / artifact / approval durability is best-effort in several transitions

**Area:** evidence trail  
**Severity:** medium

Many transitions ignore errors when appending events, creating approvals, or creating artifacts.

Examples:

- controller event appends are not checked
- review result artifact creation ignores failures
- approval creation in review flows ignores failures

That weakens the PRD requirement that important decisions be auditable and reconstructable.

### K. Review artifacts are metadata-only, not obviously persisted payloads

**Area:** evidence / artifacts  
**Severity:** medium

`CompleteReview` creates an artifact row for `reviews/<task>.json`, but this code path does not actually write the JSON payload to durable storage in the audited files.

So review evidence is recorded structurally, but not clearly materialized as a durable artifact file here.

### L. Replanning is implemented elsewhere but not exposed in the main mission command flow

**Area:** TUI mission commands  
**Severity:** medium

The PRD explicitly calls for replanning. In the audited TUI path, `/mission plan` rejects replanning for a running mission and tells the user it is not supported yet.

This is a product-level gap, not a failing test.

### M. Dashboard/store split can hide mission state when the main app falls back to memory

**Area:** TUI/dashboard consistency  
**Severity:** medium

The main TUI mission controller falls back to `InMemoryStore` if SQLite open fails. The dashboard opens SQLite only.

Implication:

- the main app may show an active mission
- the dashboard can simultaneously show no mission data

That breaks the invariant that dashboard and mission panel reflect the same durable truth.

### N. Integration ordering and verification are lighter than the PRD implies

**Area:** integration engine  
**Severity:** medium

`IntegrateReady` loops through accepted tasks and relies on dependency checks per task instead of building an explicit topological integration order. It also does not run post-merge verification in the audited path.

The current behavior is serviceable, but weaker than the PRD’s desired “deterministic integration plus post-apply validation” model.

## Coverage assessment by requested area

| Area | Current coverage | Audit result |
|---|---|---|
| Mission CRUD | Store tests, `/mission new`, `/mission list`, `/mission status`, e2e mission flow | Create/get/list/status are covered; delete is absent; update is mostly lifecycle-driven |
| Plan application | Controller tests, mission planning tests | Happy-path covered; rollback/transaction failure path is weak |
| Dependency resolution | Controller ready-resolution, scheduler tests, lifecycle/integrator tests | Core DAG flow covered; cross-mission dependency hygiene is weak |
| Run dispatch | Scheduler/worker/orchestrator tests | Concurrency is covered; running-task scope exclusion is not actually enforced against real active work |
| Review / integration | Reviewer/integrator/orchestrator tests | Good happy-path coverage; auto-accept policy and post-merge verification are weak |
| Status summaries | Mission panel tests and summary builder | Rich controller summary is solid; store summaries diverge |
| Mission commands | Unit tests and e2e | Core commands covered; replanning/recovery flows missing from main UX |
| Dashboard rendering | Dashboard unit tests and e2e launch/navigation | Rendering is covered; evidence of mission-start approval and store fallback consistency are weak |

## Recommended next verification targets before code changes

1. **Pause/cancel with an active worker** should update both run and task state durably.
2. **Pause/cancel with an active reviewer** should update review run state durably.
3. **Scheduler conflict tests using real running-task scope metadata** should fail until active-scope comparison is fixed.
4. **ApplyPlan partial-failure test** should verify transactional behavior or explicit rollback.
5. **Mission approval durability test** should require a pending `Approval` row after plan apply.
6. **Restart/recovery test through the real TUI/orchestrator start path** should verify stale leases/tasks reconcile before dispatch.
7. **Dashboard evidence test** should show mission-start approval state, not only task review approvals.
8. **Cross-mission dependency test** should ensure dependency queries require both endpoints in the same mission.
9. **Integration verification test** should enforce post-merge validation or explicitly document its absence.
10. **Store fallback consistency test** should define expected behavior when SQLite open fails.

## Bottom line

The current mission stack is already test-green and structurally coherent, but the biggest pre-implementation risks are:

1. **state inconsistency on pause/cancel/stop**
2. **scope isolation not being enforced against already-running work**
3. **non-transactional plan application**
4. **recovery/approval durability gaps versus the PRD**

Those are the edges most likely to turn future mission changes into correctness regressions.

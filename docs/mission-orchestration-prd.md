# Mission Orchestration PRD

## 1. Status

- **Product**: Golem (terminal product) + Gollem (agent framework)
- **Feature name**: Mission Orchestration / Mission Control
- **Document purpose**: implementation-ready product requirements for a state-of-the-art multi-agent mission system
- **Scope**: v1 local-first, single-host mission orchestration for repository work
- **Audience**: implementation agents, maintainers, reviewers
- **Quality bar**: the system must feel competitive with the best terminal coding agents in reliability, sophistication, and operator trust while remaining simpler and more elegant than a daemon-first platform
- **Decision rule**: if implementation behavior differs from this document, the implementation is wrong unless this document is updated first

---

## 2. Executive Summary

Golem needs a first-class **mission system** that lets a user provide one high-level objective and have the agent autonomously decompose, schedule, execute, review, and integrate many sub-goals using multiple workers.

The system must be sophisticated without becoming ornate.

The key design choice is:

- **one durable mission controller** owns truth
- **many isolated worker runs** do scoped execution
- **independent review runs** judge quality before integration
- **structured state and artifacts** are the source of truth
- **human approval gates** exist only where risk or ambiguity justifies them

The design must explicitly reject uncontrolled swarm behavior. Agents do **not** coordinate by freeform cross-chat. They coordinate through:

- mission state
- task dependencies
- task scope ownership
- structured artifacts
- verification evidence
- explicit approvals

The implementation must be:

1. **Autonomous** enough to make non-trivial progress without constant intervention.
2. **Auditable** enough that every important decision is reconstructable.
3. **Recoverable** enough to survive restarts, crashes, and worker loss.
4. **Elegant** enough that a maintainer can explain the system in one whiteboard pass.
5. **Shippable** enough that v1 is single-process and local-first.

---

## 3. Product Goals

### 3.1 Primary goals

1. **Shared-goal autonomy**
   - A user can define a large repository objective and Golem can break it into tasks, execute them in parallel where safe, and drive the mission to completion.

2. **Reliable multi-agent execution**
   - Multiple workers can run concurrently without corrupting each other’s work.
   - Parallelism comes from explicit task and scope boundaries, not from emergent conversation.

3. **Trustworthy review before integration**
   - Worker claims are not trusted by default.
   - Review is mandatory before a task may affect the integration branch/worktree.

4. **Durable mission state**
   - Missions, tasks, runs, approvals, events, and artifacts survive process restart.
   - Restart uses repository state plus durable mission state as ground truth.

5. **Operator control and comprehension**
   - The user can understand what the system is doing, why it is blocked, and what changed.
   - In the shipped surface, the user can create a mission, inspect status/tasks/dashboard state, approve the mission plan, resume via `/mission start`, pause, and cancel.
   - Task-scoped reject/retry/replan/escalation controls discussed elsewhere in this PRD remain aspirational unless a dedicated operator surface ships.

6. **Elegant implementation surface**
   - v1 must avoid unnecessary protocol, daemon, or distributed-system complexity.
   - The design must compose naturally with Golem’s existing plan, invariants, and verification workflow surfaces.

### 3.2 Non-goals

1. A fully decentralized agent swarm.
2. Multi-host distributed workers in v1.
3. A browser-first mission product in v1.
4. Hidden autonomous behavior outside visible mission state.
5. Automatic pushes, PR creation, or protected-branch merges without explicit policy.

---

## 4. Product Principles

1. **Mission truth is durable and singular**
   - The mission controller owns canonical state.
   - The transcript is useful evidence, not the primary database.

2. **Artifacts over chatter**
   - Coordination happens through task specs, diffs, test evidence, review reports, and event records.

3. **Parallelize only where ownership is clear**
   - Writable scope overlap is a scheduling failure unless policy explicitly allows it.

4. **Review is independent**
   - Review runs must treat worker output as untrusted input.

5. **Ground truth beats memory**
   - Replanning uses the repository, mission store, and artifacts, not just model recollection.

6. **Human gates only where they buy safety**
   - Every approval must correspond to a meaningful decision.
   - Approval spam is a product bug.

7. **Elegant first, distributed later**
   - v1 must not require a daemon, network protocol, or cross-process orchestration to work well.

8. **Reuse existing workflow proofing**
   - Worker and reviewer runs should use the same plan, invariants, and verification concepts already present in Golem.

---

## 5. Terminology

- **Mission**: a top-level goal with policy, budget, success criteria, and current plan state.
- **Task**: one bounded unit of mission work.
- **Run**: one actual execution attempt against one task.
- **Worker run**: a run that tries to satisfy a task.
- **Review run**: a run that judges whether a worker result meets acceptance criteria.
- **Mission controller**: the component that owns orchestration, persistence, scheduling, approvals, and recovery.
- **Task scope**: the writable file/component ownership boundary for a task.
- **Artifact**: a durable structured output attached to a mission, task, or run.
- **Lease**: time-bounded ownership of a task by a run.
- **Ready task**: task eligible for scheduling now.
- **Blocked task**: task that cannot proceed due to dependency, policy, missing information, or human gate.

---

## 6. User Stories

### 6.1 Primary user stories

1. As a developer, I can say “split the monolithic auth module into OAuth2 services, add tests, and update docs” and Golem turns that into a mission.
2. As a developer, I can watch progress across many workers without losing the thread.
3. As a developer, I can approve a mission plan and then start or resume execution with explicit lifecycle feedback.
4. As a developer, I can pause a mission, restart Golem, and continue later.
5. As a developer, I can inspect why a task failed review from durable mission evidence, even though shipped task-scoped retry/replan/escalation controls are still aspirational.
6. As a maintainer, I can prove the system works with deterministic tests and opt-in real-provider smoke tests.

### 6.2 Secondary user stories

1. As an evaluator, I can replay the event log and inspect the exact artifacts behind a mission outcome.
2. As a Gollem maintainer, I can reuse the mission primitives without the full Golem TUI.
3. As a product maintainer, I can add future background or remote execution modes without rewriting mission semantics.

---

## 7. Product Experience

### 7.1 What the user should feel

The user experience should feel like:

- one system, not five subsystems glued together
- visible parallel progress without chaos
- trustworthy review before merge
- reliable pause/resume
- clear operator leverage when things go wrong

The experience must not feel like:

- a freeform swarm
- a hidden daemon product disguised as a CLI
- a constant approval treadmill
- an opaque background process whose state only exists in model narration

### 7.2 User-facing mission phases

For the current shipped behavior, use the implementation contract below rather than older aspirational command examples.

### 7.3 Implemented mission lifecycle and operator contract

The current implementation is stricter and more explicit than the aspirational material elsewhere in this document. Treat the following as the source of truth for the shipped system:

1. **`draft`**
   - Created by `/mission new <goal>`.
   - The mission exists durably, but has no task DAG yet.
   - Operator next step: run `/mission plan`.

2. **`planning`**
   - Entered when `/mission plan` is accepted and the planner run starts.
   - Planning is rejected for vague goals; the operator must create a clearer mission goal first.
   - Re-planning is not currently supported for missions that are already `running`, `paused`, `blocked`, `completing`, or terminal.

3. **`awaiting_approval`**
   - Reached only after a plan is successfully applied.
   - Applying a plan creates durable tasks, dependencies, and a durable mission-plan approval record.
   - The mission summary intentionally distinguishes:
     - **`Awaiting approval`** when the mission-plan gate is still pending.
     - **`Ready to start`** when the plan approval is already approved and no other approvals remain.
   - `/mission start` does **not** bypass approval. If the plan approval is still pending, the user is sent to `/mission approve`.

4. **`running`**
   - Reached from `awaiting_approval` only after the durable mission-plan approval is `approved` and no other pending approvals block execution.
   - Also reached from `paused` via `/mission start`.
   - Starting a mission resumes the orchestrator loop; it does not create a second independent scheduler.

5. **`paused`**
   - Entered only from `running` via `/mission pause`.
   - Pausing stops new task leasing by stopping the in-process orchestrator.
   - Resume semantics are currently `/mission start`, not a separate `/mission resume` command.

6. **`blocked` / `completing` / terminal states**
   - These statuses exist in the durable state model and dashboard rendering.
   - The current operator-facing command flow primarily exposes create/plan/approve/start/pause/cancel/list/status/tasks.
   - `completed`, `failed`, and `cancelled` are terminal mission states.

### 7.4 Approval and start semantics

The current approval model is intentionally durable-first:

- Plan application creates a **mission-plan approval** record in the store.
- `/mission approve` resolves that durable approval to `approved`.
- After approval, the TUI immediately attempts `/mission start` behavior.
- `/mission start` succeeds only when:
  - the mission is `paused`, or
  - the mission is `awaiting_approval` **and** the mission-plan approval is durably `approved` **and** no other approvals remain pending.
- If the durable mission-plan approval record is missing, start fails with an operator-visible repair message rather than silently continuing.

This means approval and start are related but distinct:

- **Approve** = resolve the human gate durably.
- **Start** = transition the mission into active orchestration.

### 7.5 Controller, scheduler, and orchestrator responsibilities

The current implementation uses one controller-centric execution model:

- **Controller**
  - creates missions,
  - applies plans,
  - creates and resolves the durable mission-plan approval,
  - transitions mission status (`draft`, `planning`, `awaiting_approval`, `running`, `paused`, `cancelled`), and
  - derives mission summaries from durable tasks, dependencies, runs, approvals, and events.

- **Scheduler / worker launcher**
  - finds ready tasks,
  - leases them safely,
  - prepares isolated worker specs/worktrees, and
  - prevents the orchestrator from needing per-task ad hoc scheduling logic.

- **Orchestrator**
  - runs in-process on a tick loop,
  - dispatches ready workers,
  - dispatches reviewers for `awaiting_review` work,
  - integrates accepted work,
  - checks for mission completion, and
  - emits **transient orchestrator/TUI event-bus updates** such as `worker.started`, `worker.completed`, `review.pass`, `review.reject`, `review.request_changes`, `integration.completed`, `integration.failed`, and `mission.completed`.

Operationally, there is one orchestrator loop per active mission session in the TUI. The implementation does **not** expose separate user-controlled scheduler processes or a daemon-only control plane.

### 7.6 Persistence and recovery expectations

Current persistence behavior:

- Mission state is stored durably in the mission store and summarized from store data, not reconstructed from chat text.
- The dashboard opens the SQLite-backed mission store directly and can attach to existing durable mission state on startup.
- Missions persist:
  - mission metadata,
  - tasks,
  - task dependencies,
  - runs,
  - approvals,
  - events, and
  - artifacts.
- Persisted event names currently include core lifecycle and orchestration records such as `mission.created`, `plan.applied`, `mission.approved`, `mission.started`, `mission.paused`, `mission.cancelled`, `worker.dispatched`, `worker.completed`, `worker.failed`, `review.dispatched`, `review.passed`, `review.rejected`, `review.changes_requested`, `integration.completed`, `integration.conflict.requeued`, `integration.error`, `recovery.completed`, `replan.applied`, and `task.unblocked`.
- Dashboard and status views are designed to keep working after restart because they query durable state rather than transient transcript memory.

The implementation is therefore **local-first and durable-first**: operator surfaces are expected to reflect persisted mission truth, even when no active chat transcript is present.

### 7.7 Current dashboard contract

`golem dashboard` is the operator Mission Control surface for durable mission state.

Current behavior:

- If no mission ID is provided, the dashboard auto-selects the most relevant non-terminal mission with priority:
  - `running` > `blocked` > `paused` > `awaiting_approval` > `planning` > `draft`.
- The header shows mission status, task completion, active workers, pending approvals, evidence count, elapsed time, repo, branch, and worker budget.
- Pane layout is:
  - **Tasks**
  - **Workers**
  - **Evidence**
  - **Events**
- The evidence pane includes review results, pending approvals, failures, and artifacts.
- The empty-state contract is explicit: if there is no durable mission state yet, the dashboard should say **Mission Control**, **No active mission**, and instruct the operator to create one with `/mission new`. Current dashboard copy may also mention a future `golem mission new` command, but docs should treat that CLI reference as aspirational until it exists.

## 8. Scope Boundary

### 8.1 In scope for v1

- local-first, single-host orchestration
- multiple concurrent worker runs in separate git worktrees
- independent review runs
- durable mission/task/run/artifact/event storage
- mission create / plan / approve / start / pause / cancel
- resume from `paused` via start semantics
- bounded task dependency graph
- scope-based scheduling exclusion
- review-required integration
- explicit approval gates
- CLI and TUI mission surfaces
- deterministic tests and opt-in live smoke tests

### 8.2 Explicitly out of scope for v1

- remote worker pools
- browser UI
- mandatory daemon/service split
- PR automation
- issue tracker automation
- cross-repo missions
- autonomous push/deploy behavior by default

### 8.3 Deliberate extension path after v1

- optional background daemon mode
- remote worker pools
- multi-user watch/attach
- PR and CI integration
- cross-repo missions

---

## 9. System Architecture

## 9.1 High-level architecture

```text
User
  -> Golem CLI / TUI
      -> Mission Controller
          -> Planner mode
          -> Scheduler
          -> Worker Executor
          -> Review Executor
          -> Integration Engine
          -> Approval Gate
          -> SQLite Mission Store
          -> Artifact Store
          -> Worktree Manager
          -> Event Log
      -> Existing Golem / Gollem runtime
          -> models/providers
          -> tool execution
          -> plan state
          -> invariant state
          -> verification state
```

### Architectural intent

This architecture is intentionally **controller-centric**.

The controller is a real subsystem.
The planner, worker, and reviewer are mostly **execution modes** over the existing agent runtime, not separate platforms.
The integration engine is preferably deterministic code, not another always-on agent role.

That distinction is required for elegance.

## 9.2 Deployment model

v1 is **single-process and local-first**:

- `golem` launches the mission controller in-process
- worker and review runs are spawned as child mission runs managed by the controller
- durable state is written immediately to SQLite and artifact files
- on restart, the controller reconciles store state with repository state

There is **no required local daemon** in v1.

A daemon mode may be added later if and only if it materially improves:

- background durability
- multi-client attach
- remote worker pools

## 9.3 Elegance constraints

The implementation must follow these constraints:

1. There must be **one canonical state machine**, not multiple drifting ones.
2. There must be **one scheduler**, not per-role schedulers.
3. Worker and review execution should reuse the existing agent runtime as much as possible.
4. Integration should be deterministic by default and agent-assisted only when necessary.
5. The system should avoid inventing a transport protocol in v1.

---

## 10. Core Runtime Model

## 10.1 Mission Controller responsibilities

The mission controller must:

- own the mission lifecycle
- persist and load state
- request initial planning for the shipped `/mission plan` flow
- schedule runnable tasks
- lease and monitor runs
- collect artifacts
- request approvals when policy requires them
- trigger review
- integrate reviewed work
- emit events for UI and logs
- perform recovery on startup

The current implementation also contains recovery/replan-related durable state and events, but user-facing task-scoped replan controls are not yet part of the shipped operator contract.

The mission controller must **not**:

- directly do coding work itself
- accept worker claims without review
- hide side effects outside mission state

## 10.2 Execution modes

The system must support these execution modes:

### Planning mode
Used to:
- generate an initial task graph
- revise parts of the graph when new information appears
- explain why the graph changed

### Worker mode
Used to:
- complete one bounded task
- stay within assigned writable scope
- run required verification commands for its task
- produce evidence-rich output

### Review mode
Used to:
- independently inspect worker output
- rerun or supplement deterministic checks
- accept or reject the result with explicit evidence

### Integration mode
Used to:
- apply accepted worker diff into the integration worktree/branch
- run post-apply validation
- create follow-up tasks when deterministic integration is insufficient

Integration mode should be implemented as deterministic code where possible and only escalate to an agent when semantic resolution is required.

## 10.3 Why this split is the right complexity

This split is the minimum that preserves quality:

- planning is different from execution
- execution is different from review
- review is different from integration

Collapsing them into one opaque loop hurts trust and recoverability.
Turning them into many permanent platform services hurts elegance.

---

## 11. Planning Model

## 11.1 Planning output

The planner must produce a **bounded task graph**, not a speculative mega-plan.

Every task must include:

- `id`
- `title`
- `kind`
- `objective`
- `dependencies[]`
- `scope`
- `acceptance_criteria[]`
- `review_requirements[]`
- `estimated_effort`
- `risk_level`
- `blocking_reason` when applicable

### Required task kinds

- `code`
- `test`
- `docs`
- `investigation`
- `integration_followup`
- `review_fix`

## 11.2 Planning quality requirements

The planner must:

1. Prefer fewer, clearer tasks over exhaustive decomposition.
2. Separate independent writable scopes where parallelism is valuable.
3. Avoid creating “meta” tasks that only restate the mission.
4. Express acceptance criteria in observable terms.
5. Make dependencies explicit only when real.
6. Size tasks so that a strong worker can usually complete one in a single focused run.

## 11.2.1 Task sizing heuristics

Task sizing is a product-quality concern, not a minor planner detail.

A well-sized v1 task usually has:

- one clear objective
- one primary writable scope
- at most one conceptual reason for failure
- a reviewable diff size
- deterministic acceptance checks
- no hidden dependency on another still-unknown task

The planner should generally prefer a split when a proposed task would:

- touch multiple unrelated subsystems
- require both broad refactoring and behavioral verification
- span code, tests, and docs across unrelated scopes
- produce a diff so large that review becomes shallow
- combine investigation and implementation when investigation may invalidate the implementation plan

The planner should generally avoid splitting when:

- the split would create fake dependency edges with no real safety gain
- the resulting subtasks would each be too trivial to justify worker overhead
- the work must remain atomic to preserve correctness or review clarity

Strong default heuristics for v1:

- prefer 3–12 initial tasks for a non-trivial mission
- avoid more than 20 initial tasks without explicit structural-replan approval
- prefer one writable scope per task
- prefer one review run per completed worker task
- prefer follow-up tasks over speculative front-loading

**Aspirational note:** the remainder of this replanning section describes desired future policy and recovery behavior, not the current shipped operator surface. Today the shipped user-facing commands remain centered on `/mission new|status|tasks|plan|approve|start|pause|cancel|list` plus `golem dashboard`; there is no shipped `/mission replan`, `/mission retry`, or escalation command.

## 11.3 Replanning triggers

Replanning must be triggered when any of the following occurs:

- a review run rejects a task
- integration reveals a semantic conflict
- a worker reports a blocker with evidence
- repository state drifts from the mission assumption baseline
- mission budget or policy changes materially
- no runnable tasks remain and mission is not complete

## 11.4 Replanning policy

Every replan must produce a durable **replan artifact** containing:

- `replan_id`
- `trigger_type`
- `trigger_event_ids[]`
- `reason_summary`
- `before_graph_summary`
- `after_graph_summary`
- `task_delta`
- `scope_delta`
- `budget_delta`
- `requires_approval`
- `superseded_task_ids[]`
- `created_at`

A replan is **local** only if all of the following are true:

1. it does not invalidate already accepted or done work
2. it does not widen any task from narrow scope to `repo_wide`
3. it adds at most 3 new tasks and removes at most 3 not-yet-started tasks
4. it does not materially increase mission budget
5. it does not change mission success criteria
6. it does not cancel an active run that is still operating inside a valid scope

A replan is **structural** if any of the above is false.

### Local replan
Typical local replans include:
- adding 1–3 follow-up tasks after review rejection
- splitting one blocked task into a few narrower tasks
- tightening acceptance criteria after new deterministic evidence
- requeueing or blocking a task with an updated reason

Local replans may auto-apply if mission policy allows.
The controller must still emit a visible event and persist a before/after delta.

### Structural replan
Structural replans include:
- replacing the active task set
- changing major scope boundaries
- materially increasing expected cost or runtime
- changing the meaning of mission completion
- invalidating currently active work

Structural replans must require approval by default.

### Structural replan execution rules
When a structural replan is proposed:

1. the controller stops leasing new tasks immediately
2. the controller classifies active runs as:
   - `continue_allowed`
   - `safe_to_cancel`
   - `must_finish_before_replan`
3. the controller presents one approval decision surface with the complete delta
4. on approval, superseded ready tasks are marked `superseded` and new tasks are inserted transactionally
5. on rejection, the current plan remains canonical and execution resumes from the existing graph

The implementation must not partially apply a structural replan.

---

## 12. Scheduling Model

## 12.1 Scheduling inputs

The scheduler uses:

- task readiness
- task priority
- dependency completion
- scope overlap rules
- active worker count
- review backlog
- mission budget
- approval status

## 12.2 Scheduling rules

1. Only schedule tasks in `ready` state.
2. Never lease the same task twice concurrently.
3. Never schedule overlapping writable scopes concurrently unless policy explicitly permits it.
4. Read-only investigation or review tasks may overlap writable tasks if they do not mutate scope.
5. Prefer tasks with:
   - higher priority
   - narrower scope
   - lower merge risk
   - lower estimated runtime when the queue is saturated
6. Reserve review capacity when completed worker results are waiting.

## 12.3 Task scopes

Each task scope must be one of:

- `paths`: glob/path set
- `components`: named subsystem ownership labels
- `repo_wide`: only for explicitly approved wide-scope tasks

v1 should implement `paths` first and may add `components` as metadata layered on top.

## 12.4 Leases and heartbeats

Every active run must hold a lease.

Lease rules:

- lease is acquired when a task is assigned
- run heartbeats extend the lease
- lease expiry returns the task to `ready` or `blocked` after reconciliation
- lease loss is recorded as a run terminal state

The scheduler must be safe against duplicate assignment under restart or slow cleanup.

---

## 13. Isolation and Git Model

## 13.1 Worktree strategy

Every worker and review run gets:

- its own git worktree
- its own output directory
- its own logs
- its own ephemeral runtime state if needed

The controller also owns an **integration worktree** or mission branch workspace.

## 13.2 Repository baseline

At mission start, the controller must record:

- repository root
- HEAD commit
- dirty working tree status
- current branch
- whether the mission is allowed to run on a dirty repo

Default policy:
- refuse mission start on a dirty repo unless the user explicitly approves

## 13.3 Merge strategy

Integration flow:

1. accepted task result is applied into the integration worktree
2. post-apply validation runs
3. if clean, the task becomes `done`
4. if deterministic conflict or fallout occurs, create follow-up tasks or request approval

The system must not directly apply worker changes into the user’s primary working tree during execution.

## 13.4 Conflict policy

Conflicts are handled as follows:

1. **No conflict**: integrate normally.
2. **Deterministic mechanical conflict**: may auto-resolve only with tested logic.
3. **Semantic conflict**: create a follow-up task or request approval.

A semantic conflict must never be silently auto-resolved.

---

## 14. Review Model

## 14.1 Review is mandatory

A task that changes repository state must not integrate until a review run marks it `pass`.

Investigation-only tasks may use a lighter review contract if they produce no writable diff, but they still require recorded evidence.

## 14.2 Review inputs

A review run receives:

- the task spec
- the worker result summary
- the worker diff
- worker verification results
- current repository context
- any relevant mission policy

## 14.3 Review requirements

The review run must check:

- task objective completion
- acceptance criteria satisfaction
- declared commands actually ran
- claimed tests actually passed
- writable scope adherence
- new diagnostics or regressions
- unsupported claims or missing evidence

## 14.4 Review outcomes

A review result is one of:

- `pass`
- `fail`
- `partial`

### `pass`
- task may move to integration

### `fail`
- task returns to planning/retry path
- result must include explicit rejection reasons

### `partial`
- task may split into targeted follow-up tasks
- original worker diff does not integrate yet

---

## 15. Mission Data Model

The persistence model must be simple enough to reason about and rich enough to recover from failure.

SQLite is the source of truth for structured mission state.
Artifacts on disk are the source of truth for large evidence blobs.

## 15.1 Storage layout

```text
~/.golem/
  missions/
    missions.db
    artifacts/
      <mission-id>/
        <run-id>/
          task_spec.json
          run_summary.json
          diff.patch
          verification.json
          review.json
          integration.json
```

## 15.2 Tables

The SQL below is normative for v1 shape, though column names may vary slightly if equivalent semantics are preserved.

### `missions`

```sql
CREATE TABLE missions (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  goal TEXT NOT NULL,
  repo_root TEXT NOT NULL,
  base_commit TEXT NOT NULL,
  base_branch TEXT NOT NULL,
  status TEXT NOT NULL,
  policy_json TEXT NOT NULL,
  budget_json TEXT NOT NULL,
  success_criteria_json TEXT NOT NULL,
  integration_ref TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  started_at TIMESTAMP,
  ended_at TIMESTAMP,
  last_replan_at TIMESTAMP,
  CHECK (status IN (
    'draft','planning','awaiting_approval','running','paused','blocked','completing','completed','failed','cancelled'
  ))
);

CREATE INDEX idx_missions_status ON missions(status);
CREATE INDEX idx_missions_updated_at ON missions(updated_at DESC);
```

### `tasks`

```sql
CREATE TABLE tasks (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  title TEXT NOT NULL,
  kind TEXT NOT NULL,
  objective TEXT NOT NULL,
  status TEXT NOT NULL,
  priority INTEGER NOT NULL,
  scope_json TEXT NOT NULL,
  acceptance_criteria_json TEXT NOT NULL,
  review_requirements_json TEXT NOT NULL,
  estimated_effort TEXT NOT NULL,
  risk_level TEXT NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  blocking_reason TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE,
  CHECK (status IN (
    'pending','ready','leased','running','awaiting_review','accepted','integrated','done','rejected','failed','blocked','superseded'
  )),
  CHECK (priority >= 0)
);

CREATE INDEX idx_tasks_mission_status ON tasks(mission_id, status);
CREATE INDEX idx_tasks_mission_priority ON tasks(mission_id, priority DESC, created_at ASC);
```

### `task_dependencies`

```sql
CREATE TABLE task_dependencies (
  task_id TEXT NOT NULL,
  depends_on_task_id TEXT NOT NULL,
  PRIMARY KEY (task_id, depends_on_task_id),
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  FOREIGN KEY (depends_on_task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CHECK (task_id != depends_on_task_id)
);

CREATE INDEX idx_task_dependencies_depends_on ON task_dependencies(depends_on_task_id);
```

### `runs`

```sql
CREATE TABLE runs (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  lease_owner TEXT,
  lease_expires_at TIMESTAMP,
  heartbeat_at TIMESTAMP,
  worktree_path TEXT NOT NULL,
  started_at TIMESTAMP,
  ended_at TIMESTAMP,
  summary TEXT,
  error_text TEXT,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE,
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CHECK (mode IN ('planner','worker','review','integration')),
  CHECK (status IN ('queued','running','succeeded','failed','timed_out','cancelled','lease_lost'))
);

CREATE INDEX idx_runs_mission_status ON runs(mission_id, status);
CREATE INDEX idx_runs_task_id ON runs(task_id);
CREATE INDEX idx_runs_lease_expires_at ON runs(lease_expires_at);
```

### `artifacts`

```sql
CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  task_id TEXT,
  run_id TEXT,
  type TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE,
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL,
  FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL,
  UNIQUE (run_id, type, relative_path)
);

CREATE INDEX idx_artifacts_mission_id ON artifacts(mission_id);
CREATE INDEX idx_artifacts_task_id ON artifacts(task_id);
CREATE INDEX idx_artifacts_run_id ON artifacts(run_id);
```

### `approvals`

```sql
CREATE TABLE approvals (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL,
  task_id TEXT,
  run_id TEXT,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  request_json TEXT NOT NULL,
  response_json TEXT,
  created_at TIMESTAMP NOT NULL,
  resolved_at TIMESTAMP,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE,
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL,
  FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL,
  CHECK (status IN ('pending','approved','rejected','superseded','expired'))
);

CREATE INDEX idx_approvals_mission_status ON approvals(mission_id, status);
```

### `events`

```sql
CREATE TABLE events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  mission_id TEXT NOT NULL,
  task_id TEXT,
  run_id TEXT,
  type TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  FOREIGN KEY (mission_id) REFERENCES missions(id) ON DELETE CASCADE,
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL,
  FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL
);

CREATE INDEX idx_events_mission_id_id ON events(mission_id, id);
CREATE INDEX idx_events_task_id ON events(task_id);
CREATE INDEX idx_events_run_id ON events(run_id);
```

## 15.2.1 Persistence rules

The store layer must guarantee:

1. task leasing is transactional
2. structural replan application is transactional
3. accepted-to-integrated transitions are transactional
4. event append happens in the same transaction as the state change it represents where practical
5. artifact file writes happen before the DB row is committed, or the row must be rolled back
6. recovery never depends on in-memory-only state

## 15.2.2 Event taxonomy

The event log is not a dumping ground. Event types must be explicit and low-cardinality.

For the current shipped system, distinguish between:

1. **Persisted mission-store event names** — written to the durable `events` table and rendered by the dashboard.
2. **Transient orchestrator/TUI event-bus names** — emitted by the in-process orchestrator to update the TUI chat stream.

### Persisted mission lifecycle/store event examples
- `mission.created`
- `plan.applied`
- `mission.approved`
- `mission.started`
- `mission.paused`
- `mission.cancelled`
- `mission.completed`

### Persisted worker/review/integration/recovery examples
- `worker.dispatched`
- `worker.completed`
- `worker.failed`
- `worker.cancelled`
- `worker.lease_lost`
- `worker.provision_failed`
- `review.dispatched`
- `review.passed`
- `review.rejected`
- `review.changes_requested`
- `review.failed`
- `review.auto_accepted`
- `review.provision_failed`
- `integration.completed`
- `integration.conflict.requeued`
- `integration.error`
- `recovery.completed`
- `recovery.stuck_task_reset`
- `replan.requested`
- `replan.applied`
- `task.unblocked`
- `worktree.release_failed`

### Transient orchestrator/TUI event-bus examples
- `worker.started`
- `worker.completed`
- `worker.failed`
- `worker.spawn_failed`
- `review.started`
- `review.pass`
- `review.reject`
- `review.request_changes`
- `review.failed`
- `review.parse_failed`
- `integration.completed`
- `integration.failed`
- `mission.completed`

Dashboard rendering should tolerate future additive event types, but persisted examples in this section should use the durable store names above because those are the names the dashboard/store currently persist. Transient orchestrator/TUI names should only be used when explicitly discussing the in-memory event bus.

Event payload rules:

1. every event payload must be schema-stable within a schema version
2. event names must describe facts, not UI actions
3. replaying the event stream should explain mission history without requiring hidden runtime context

## 15.3 State machines

### Mission states

```text
draft -> planning -> awaiting_approval -> running -> paused -> running
                                              |         |        |
                                              |         |        -> blocked
                                              |         |        -> completing
                                              |         |        -> failed
                                              |         |        -> cancelled
                                              |         -> cancelled
                                              -> cancelled
completing -> completed | failed
blocked -> planning | awaiting_approval | cancelled
```

### Task states

```text
pending -> ready -> leased -> running -> awaiting_review -> accepted -> integrated -> done
                      |         |            |               |
                      |         |            |               -> rejected
                      |         |            -> failed
                      |         -> lease_lost
                      -> blocked

rejected -> ready | blocked | pending
failed   -> ready | blocked | pending
integrated -> done | blocked
```

### Run states

```text
queued -> running -> succeeded
                -> failed
                -> timed_out
                -> cancelled
                -> lease_lost
```

---

## 16. Artifact Model

Artifacts must be **system-generated whenever possible**.

The system should not depend on the worker manually remembering to write files correctly.

## 16.1 Required artifacts per task attempt

### `task_spec.json`
Generated by the controller.

Required fields:

- `mission_id`
- `task_id`
- `title`
- `objective`
- `constraints[]`
- `scope`
- `acceptance_criteria[]`
- `review_requirements[]`
- `verification_commands[]`
- `policy`
- `context_summary`

### `run_summary.json`
Generated by the controller from structured run state plus final agent summary.

Required fields:

- `mission_id`
- `task_id`
- `run_id`
- `mode`
- `final_summary`
- `claims[]`
- `files_changed[]`
- `commands_run[]`
- `verification_commands_run[]`
- `risks[]`
- `blockers[]`
- `suggested_followups[]`

### `diff.patch`
Generated by the system from the run worktree diff against the run base revision.

Required when the run changes repository contents.

### `verification.json`
Generated from verification tool state and recorded command results.

Required fields:

- `commands[]`
- `results[]`
- `overall_result`
- `stdout_refs[]`
- `stderr_refs[]`

### `review.json`
Generated from review run output.

Required fields:

- `task_id`
- `run_id`
- `result`
- `checks[]`
- `evidence[]`
- `rejection_reasons[]`
- `recommended_followups[]`

### `integration.json`
Generated by the integration engine.

Required fields:

- `task_id`
- `result`
- `apply_method`
- `post_apply_checks[]`
- `followup_tasks[]`
- `conflicts[]`

## 16.2.1 Artifact JSON schema expectations

The implementation should validate artifact structure at write time.

Minimum schema expectations:

### `task_spec.json`
- `mission_id`: string
- `task_id`: string
- `title`: string
- `objective`: string
- `constraints`: array[string]
- `scope`: object
- `acceptance_criteria`: array[string]
- `review_requirements`: array[string]
- `verification_commands`: array[string]
- `policy`: object
- `context_summary`: string

### `run_summary.json`
- `mission_id`: string
- `task_id`: string
- `run_id`: string
- `mode`: string
- `final_summary`: string
- `claims`: array[string]
- `files_changed`: array[string]
- `commands_run`: array[string]
- `verification_commands_run`: array[string]
- `risks`: array[string]
- `blockers`: array[string]
- `suggested_followups`: array[string]

### `verification.json`
- `commands`: array[string]
- `results`: array[object]
- `overall_result`: string
- `stdout_refs`: array[string]
- `stderr_refs`: array[string]

### `review.json`
- `task_id`: string
- `run_id`: string
- `result`: `pass|fail|partial`
- `checks`: array[object]
- `evidence`: array[object|string]
- `rejection_reasons`: array[string]
- `recommended_followups`: array[string]

### `integration.json`
- `task_id`: string
- `result`: string
- `apply_method`: string
- `post_apply_checks`: array[object|string]
- `followup_tasks`: array[object|string]
- `conflicts`: array[object|string]

Validation policy:

1. malformed required artifact JSON must fail the associated run stage
2. schema-valid but semantically insufficient artifacts must fail review, not persistence
3. artifact schema validation errors must be visible in mission state and event log

## 16.2.2 Artifact naming conventions

Recommended stable names per run:

- `task_spec.json`
- `run_summary.json`
- `diff.patch`
- `verification.json`
- `review.json`
- `integration.json`
- `replan.json` (when applicable)

## 16.2.3 Concrete artifact examples

The documentation should include representative artifact examples so implementers and reviewers can align on shape.

### Example `task_spec.json`

```json
{
  "mission_id": "m_auth_oauth2",
  "task_id": "t_callback_handler",
  "title": "Implement OAuth callback handler",
  "objective": "Add the server-side callback endpoint and token exchange flow.",
  "constraints": [
    "Do not widen scope beyond internal/auth and related tests.",
    "Do not modify frontend login UI in this task."
  ],
  "scope": {
    "paths": ["internal/auth/**", "internal/auth_test/**"]
  },
  "acceptance_criteria": [
    "Callback endpoint exists.",
    "Token exchange path is covered by tests.",
    "go test ./... passes."
  ],
  "review_requirements": [
    "Confirm no writes outside scope.",
    "Confirm tests cited by the worker actually ran."
  ],
  "verification_commands": [
    "go test ./internal/auth/...",
    "go test ./..."
  ],
  "policy": {
    "network": false,
    "shell": true
  },
  "context_summary": "Mission is splitting auth migration into narrow backend-first tasks."
}
```

### Example `review.json`

```json
{
  "task_id": "t_callback_handler",
  "run_id": "r_review_17",
  "result": "fail",
  "checks": [
    {"name": "scope adherence", "result": "pass"},
    {"name": "tests pass", "result": "fail"}
  ],
  "evidence": [
    {"kind": "command", "value": "go test ./..."},
    {"kind": "stderr_ref", "value": "artifacts/r_review_17/go-test.stderr.txt"}
  ],
  "rejection_reasons": [
    "Worker claimed end-to-end success but repository-wide tests still fail."
  ],
  "recommended_followups": [
    "Fix remaining test failure in auth token refresh path."
  ]
}
```

## 16.3 Artifact generation requirements

1. Artifact generation must be deterministic when possible.
2. Missing required artifacts must block task acceptance.
3. Artifact file names and paths must be deterministic.
4. Artifacts must be referenced from the database by hash and path.

---

## 17. Mission Workflow

## 17.1 Mission creation

1. User creates mission from CLI or TUI.
2. Controller validates repository preconditions.
3. Mission is created in `draft` state.
4. Planning mode runs.
5. Tasks and dependencies are persisted transactionally.
6. Mission moves to `planning` then `awaiting_approval` or `running` depending on policy.

## 17.2 Execution loop

1. Scheduler selects runnable tasks.
2. Controller leases tasks and creates worktrees.
3. Worker runs execute.
4. Required artifacts are generated.
5. Task moves to `awaiting_review` if worker succeeded, else to retry/block path.
6. Review run executes.
7. If review passes, task moves to `accepted`.
8. Integration engine applies the change.
9. Post-apply checks run.
10. Task becomes `done` or follow-up work is created.
11. Controller checks completion, blocking, or replanning conditions.

## 17.3 Pause and resume

### Soft pause
- stop leasing new tasks
- let active runs finish
- mark mission `paused` when no active runs remain

### Hard pause
- stop leasing new tasks
- cancel active runs
- reconcile partial state
- mark mission `paused`

### Resume
- reconcile expired leases
- reclaim abandoned tasks
- continue scheduling from durable truth

## 17.4 Crash recovery

On startup, the controller must:

1. open the mission store
2. identify active missions not terminally resolved
3. detect runs with expired leases or dead child processes
4. mark those runs `lease_lost`
5. regenerate missing derived state from repo + artifacts where possible
6. move affected tasks to `ready` or `blocked`
7. emit recovery events

---

## 18. Policy, Budget, and Approval Model

## 18.1 Mission budget

Mission budget must support:

- `max_concurrent_workers`
- `max_total_runs`
- `max_model_calls`
- `max_cost_usd`
- `max_wall_clock_duration`
- `max_replans`
- `max_consecutive_failures`

The controller must check budget before every new lease.

## 18.2 Task budget

Task policy may override:

- max runtime
- max retries
- max changed files
- max shell commands
- max tool calls

## 18.3 Approval events

Current shipped operator behavior centers on a durable **mission-plan approval** gate:

- applying a plan creates the mission-plan approval record
- `/mission approve` resolves that durable gate
- `/mission start` can begin execution only after the plan approval is durably approved
- if any other approval records remain pending, the mission summary and start flow surface them as blockers

Additional approval kinds may exist in the durable model, but the operator-facing command flow currently guarantees and documents the plan-approval gate above.

## 18.4 Approval lifecycle semantics

The durable approval model supports approval records with these fields:

- a stable `approval_id`
- `kind`
- `status` in `pending|approved|rejected|superseded|expired`
- `mission_id`
- optional `task_id` and `run_id`
- `decision_surface_json`
- `requested_at`
- `resolved_at`
- optional `superseded_by`

Current shipped operator flow guarantees only the mission-plan approval path described above. The broader rejection/supersession semantics below describe durable model expectations and future-proof behavior, not an additional shipped user command surface.

Controller rules:

1. there must be at most one active blocking approval for the same decision surface
2. repeated identical approval requests must coalesce instead of spamming the operator
3. a new approval that makes an older one obsolete must mark the older request `superseded`
4. approvals must be durable across restart
5. if a future operator surface supports rejection, rejecting an approval must produce a deterministic next state:
   - resume current plan
   - block task
   - cancel risky action
   - or fail mission if no safe path remains

## 18.5 Approval UX requirement

Every approval request must present:

- exact action requested
- why approval is needed
- affected mission/task/run
- visible evidence to inform the decision
- blast radius / writable scope impact
- what is currently paused while waiting
- explicit consequences of approve vs reject

Approval UX must also satisfy:

1. the operator can inspect the relevant diff, summary, and test evidence without leaving the mission flow
2. the default focused action must be the safest reasonable option
3. approval screens must be stable enough to survive restart without ambiguity
4. approval volume must stay low; approval spam is a product bug

If the approval UI cannot explain the decision surface clearly, the product design is wrong.

## 18.5.1 Concrete approval decision-surface example

Representative `decision_surface_json` for a structural replan:

```json
{
  "kind": "structural_replan",
  "mission_id": "m_auth_oauth2",
  "task_id": "t_auth_refactor",
  "reason": "Review failure revealed a hidden dependency between token persistence and callback handling.",
  "requested_action": "Replace 2 active planned tasks with 3 narrower tasks and supersede 1 blocked task.",
  "paused_work": [
    "No new leases will be issued until decision is made."
  ],
  "blast_radius": {
    "scope_delta": ["internal/auth/**"],
    "budget_delta": {"estimated_extra_runs": 2}
  },
  "approve_consequences": [
    "Current task graph is replaced transactionally.",
    "Superseded tasks are marked superseded."
  ],
  "reject_consequences": [
    "Existing graph remains active.",
    "Mission resumes from current plan."
  ],
  "evidence_refs": [
    "artifacts/m_auth_oauth2/r_review_17/review.json",
    "artifacts/m_auth_oauth2/r_plan_9/replan.json"
  ]
}
```

## 18.6 Failure and retry policy matrix

The controller must treat failures explicitly rather than burying them in generic retry logic.

| Failure case | Default action | Auto-retry? | Replan? | Approval? |
|---|---|---:|---:|---:|
| worker tool/runtime error | requeue task if retry budget remains | yes | no | no |
| worker exceeded task budget | block task or requeue with tighter scope | limited | maybe | no |
| review `fail` with clear fix path | create retry or follow-up task | no direct blind retry | maybe | no |
| review `partial` | create follow-up task(s) | no | yes | no |
| integration deterministic conflict | create integration follow-up task | no | maybe | no |
| integration semantic conflict | stop integration path | no | yes | maybe |
| lease lost / worker disappeared | reclaim task and requeue after reconciliation | yes | no | no |
| repeated same-task failures beyond threshold | block task | no | yes | maybe |
| structural replan proposal | pause new leases until decision | n/a | yes | yes |
| writable scope escalation | halt that task path | no | no | yes |

Required retry rules:

1. retries must increment visible attempt counters
2. retries must preserve prior evidence/artifacts
3. repeated identical failures must push the mission toward replan/block, not infinite retry loops
4. retry budget exhaustion must be visible in the task state

## 18.7 Human approval decision matrix

The implementation must make approval policy explicit enough that a coding agent can encode it directly.

| Decision surface | Default | Rationale |
|---|---|---|
| initial plan | approval required unless auto-start policy enabled | operator should confirm mission framing |
| local replan | auto-approve if policy allows | keeps operator out of low-risk bookkeeping |
| structural replan | approval required | graph meaning changes materially |
| scope escalation to `repo_wide` | approval required | blast radius increases sharply |
| high-risk shell/network escalation | approval required | operational/safety risk |
| deterministic integration into mission branch/worktree | configurable | some teams want review before integration |
| final mission completion | configurable | some teams require sign-off on success claim |

UI requirement:

The approval queue should show the decision surface kind as a first-class label so the operator can tell whether they are approving a plan, a replan, an escalation, or final completion.

## 19. Prompting and Role Contracts

## 19.1 Shared contract for all mission runs

Every mission run must inherit core Golem discipline:

- read before edit
- create/update a plan for non-trivial work
- maintain invariant/evidence discipline
- verify before claiming completion
- no unsupported claims
- no TODO placeholders

## 19.2 Worker contract

A worker run must:

- focus on exactly one task
- stay within assigned writable scope
- keep task-local plan state accurate
- produce evidence for claims
- run required verification commands where appropriate
- report blockers explicitly
- never claim that the work is accepted or merged

### Worker prompt template

The implementation should use a stable prompt shape similar to:

```text
You are a mission worker operating inside Golem Mission Control.

You own exactly one task.
Your task is not complete until the task-local acceptance criteria are satisfied and the required evidence artifacts exist.

Task:
- id: {{task_id}}
- title: {{title}}
- objective: {{objective}}
- writable scope: {{scope}}
- acceptance criteria:
{{acceptance_criteria}}
- review requirements:
{{review_requirements}}

Rules:
1. Stay within writable scope unless explicitly escalated.
2. Read before editing.
3. Keep a short task-local plan.
4. Run the required verification commands.
5. Make no unsupported claims.
6. Do not say the work is accepted, merged, or complete for the mission.
7. If blocked, stop and report the blocker with concrete evidence.

Required outputs:
- updated task-local plan state
- diff or no-op proof
- run summary
- verification evidence
```

## 19.3 Review contract

A review run must:

- treat worker output as untrusted
- validate claims against artifacts and repository state
- reject unsupported claims
- produce explicit pass/fail evidence
- recommend next action when rejecting

### Review prompt template

```text
You are an independent mission reviewer.

Your job is not to continue the worker's implementation.
Your job is to decide whether the submitted result satisfies the task contract.

Inputs:
- task spec
- worker summary
- diff / no-op proof
- verification evidence
- current repository state

Rules:
1. Treat the worker summary as untrusted input.
2. Prefer deterministic evidence over narration.
3. Reject unsupported claims.
4. If the result is insufficient, explain exactly why.
5. Do not silently repair the work during review.

Required outputs:
- review result: pass | fail | partial
- checks performed
- evidence
- rejection reasons or follow-up recommendations
```

## 19.4 Planning contract

A planning run must:

- create bounded tasks
- minimize writable scope overlap
- avoid unnecessary task explosion
- separate execution from review concerns
- explain any structural replan

### Planner prompt template

```text
You are the mission planner.

Produce a bounded task graph for the mission.
Favor clarity, ownership boundaries, and reviewability over exhaustive decomposition.

Mission goal:
{{mission_goal}}

Constraints:
{{mission_constraints}}

Rules:
1. Prefer fewer, clearer tasks.
2. Keep writable scopes narrow.
3. Make dependencies explicit only when real.
4. Separate implementation, review, and follow-up work.
5. If proposing a structural replan, explain the delta from the current graph.
6. Avoid speculative tasks without clear evidence.

Required outputs:
- task graph
- task scopes
- acceptance criteria per task
- replan explanation when applicable
```

---

## 20. CLI and TUI Surface

## 20.1 CLI and TUI surfaces

Current shipped operator surfaces are:

- slash commands inside the main TUI via `/mission new|status|tasks|plan|approve|start|pause|cancel|list`
- the standalone `golem dashboard [mission-id]` Mission Control surface

Implementation notes:

- there is no separate shipped `/mission resume` slash command; resume is performed with `/mission start` from `paused`
- the current docs for JSON envelopes and a broader `golem mission ...` subcommand family remain aspirational unless and until that CLI surface is implemented

## 20.1.1 TUI mission commands

Current TUI help contract:

```text
/mission new <goal>
/mission status
/mission tasks
/mission plan
/mission approve
/mission start
/mission pause
/mission cancel
/mission list
```

Semantics:

- `/mission new <goal>` creates the mission in `draft`
- `/mission plan` moves the mission into `planning` and later applies the DAG into durable store state
- `/mission approve` resolves the durable mission-plan approval and then attempts to start execution
- `/mission start` starts an approved `awaiting_approval` mission or resumes a `paused` mission
- `/mission pause` stops new task leasing
- `/mission status` and `/mission tasks` read durable mission state
- `/mission list` shows known missions and marks the currently active one in the TUI session

## 20.1.2 Dashboard surface

Current dashboard contract:

- `golem dashboard [mission-id]` opens Mission Control against the durable mission store
- if no mission ID is supplied, the dashboard selects the highest-priority non-terminal mission (`running` > `blocked` > `paused` > `awaiting_approval` > `planning` > `draft`)
- the surface renders four panes: **Tasks**, **Workers**, **Evidence**, and **Events**
- if no durable mission exists, the empty state should still render Mission Control and guide the operator toward `/mission new`; any broader `golem mission ...` CLI guidance remains aspirational until that CLI ships

## 20.1.3 Aspirational CLI JSON contracts

The broader `golem mission ... --json` contracts below remain aspirational unless and until that dedicated CLI family is implemented.

All `--json` mission commands must emit stable top-level envelopes.

Minimum envelope:

```json
{
  "version": "1",
  "command": "mission status",
  "data": {}
}
```

Required command contracts:

### `golem mission status <mission-id> --json`

```json
{
  "version": "1",
  "command": "mission status",
  "data": {
    "mission": {
      "id": "m_auth_oauth2",
      "title": "OAuth2 migration",
      "status": "running",
      "budget": {"max_concurrent_workers": 3},
      "success_criteria": ["tests pass"]
    },
    "counts": {
      "ready_tasks": 2,
      "running_tasks": 1,
      "pending_approvals": 0
    }
  }
}
```

### `golem mission tasks <mission-id> --json`

```json
{
  "version": "1",
  "command": "mission tasks",
  "data": {
    "mission_id": "m_auth_oauth2",
    "tasks": [
      {
        "id": "t_callback_handler",
        "title": "Implement OAuth callback handler",
        "status": "awaiting_review",
        "priority": 90
      }
    ]
  }
}
```

### `golem mission approval show <approval-id> --json`

```json
{
  "version": "1",
  "command": "mission approval show",
  "data": {
    "approval": {
      "id": "a_replan_3",
      "kind": "structural_replan",
      "status": "pending",
      "decision_surface": {}
    }
  }
}
```

JSON contract rules:

1. command output must not silently change field meaning across minor revisions
2. omitted fields must mean truly unavailable, not implicitly false
3. human-readable CLI output may evolve freely, but `--json` must stay stable enough for tests and tooling

## 20.2 TUI mission board

**Aspirational note:** the detailed mission-board model in this section describes a richer future mission workspace than the currently shipped surface. Today the shipped operator experience is the `/mission ...` chat flow plus `golem dashboard`, with durable summaries, task lists, and the four Mission Control panes.

The future TUI mission board should provide:

- mission overview panel
- task board with dependency visibility
- active run list
- review queue
- approval queue
- event log
- budget/cost panel
- selected task detail
- selected run artifact detail

## 20.2.1 Primary mission screens

The mission UX should be implemented as a small set of stable screens/panels, not a maze of modal states.

Required primary views:

1. **Mission overview**
   - mission title, status, budget, active phase, success criteria summary
2. **Task board**
   - task columns by state with dependency hints and ownership/scope visibility
3. **Run inspector**
   - active and recent runs with lease status, timestamps, and summaries
4. **Approval view**
   - one explicit decision surface with evidence and consequences
5. **Artifact inspector**
   - diff, summary, test evidence, review evidence, replan delta
6. **Event timeline**
   - append-only replayable mission history

## 20.2.2 Key interaction flows

### Create mission flow
1. capture goal/title/policy
2. validate repo preconditions
3. create draft mission
4. move into planning state and then overview/approval flow

### Review rejection flow
1. operator sees rejected task in board and review queue
2. selecting the task opens worker summary, diff, and review evidence
3. operator can choose retry, force replan, or cancel mission path

### Approval flow
1. approval queue highlights blocking request
2. approval detail screen shows requested action, evidence, paused work, and consequences
3. operator can approve or reject without losing surrounding mission context

### Completion flow
1. overview shows mission in completing/completed state
2. artifact inspector exposes final integration evidence
3. event timeline makes the mission auditable end-to-end

## 20.2.3 Interaction quality requirements

The TUI must:

- preserve operator context while moving between board, run detail, and approval detail
- make the currently blocking condition visually obvious
- allow the user to inspect evidence before making approval decisions
- avoid hiding essential mission status behind nested modal stacks
- remain usable during high event throughput

## 20.3 Required operator actions

### 20.3.1 Shipped operator actions

From the shipped TUI and dashboard surfaces, the operator can currently:

- create a mission with `/mission new <goal>`
- inspect durable mission status with `/mission status`
- inspect the task DAG with `/mission tasks`
- invoke planning with `/mission plan`
- approve the durable mission-plan gate with `/mission approve`
- start or resume execution with `/mission start`
- pause a running mission with `/mission pause`
- cancel a mission with `/mission cancel`
- list missions with `/mission list`
- inspect Mission Control state in `golem dashboard`
- inspect final or in-progress evidence through durable mission status, dashboard evidence, events, and artifacts

### 20.3.2 Aspirational operator actions

The following actions are still aspirational in this PRD unless and until a dedicated surface ships:

- reject the plan from a user-facing mission command
- retry a task directly from a user-facing mission command
- force a replan from a user-facing mission command
- approve/reject escalation from a dedicated operator control surface

## 20.4 TUI state model

The TUI mission surface should use a small explicit state model.

Illustrative view state:

```go
type MissionScreen string

const (
    MissionScreenBoard      MissionScreen = "board"
    MissionScreenTaskDetail MissionScreen = "task_detail"
    MissionScreenRunDetail  MissionScreen = "run_detail"
    MissionScreenApproval   MissionScreen = "approval"
    MissionScreenArtifacts  MissionScreen = "artifacts"
    MissionScreenTimeline   MissionScreen = "timeline"
)

type MissionUIState struct {
    MissionID          string
    ActiveScreen       MissionScreen
    SelectedTaskID     string
    SelectedRunID      string
    SelectedApprovalID string
    BoardFilter        string
    TaskCursor         int
    RunCursor          int
    TimelineCursor     int
    PendingAction      string
    LastError          string
}
```

## 20.5 TUI message/event contracts

The mission UI should react to explicit messages rather than inferring transitions from render state.

Minimum message families:

```go
type MissionLoadedMsg struct{ Mission MissionViewModel }
type MissionEventAppendedMsg struct{ Event MissionEventViewModel }
type TaskSelectedMsg struct{ TaskID string }
type RunSelectedMsg struct{ RunID string }
type ApprovalSelectedMsg struct{ ApprovalID string }
type ApprovalResolvedMsg struct{ ApprovalID string; Status string }
type MissionActionRequestedMsg struct{ Action string; MissionID string }
type MissionActionCompletedMsg struct{ Action string; MissionID string; Err error }
type MissionScreenChangedMsg struct{ Screen MissionScreen }
```

## 20.6 TUI state machine requirements

The TUI must obey these rules:

1. a blocking approval should automatically surface in the approval queue and be reachable in one action from the board
2. task selection should remain stable across unrelated event updates when possible
3. switching between board/detail views must preserve selection context
4. destructive actions must require confirmation or use an approval-style decision surface
5. render state must derive from canonical mission data, not mutate canonical mission state directly

---

## 21. Repository Boundaries

## 21.1 Gollem responsibilities

Gollem should own reusable mission primitives:

- mission/task/run state types
- store interfaces and SQLite implementation
- scheduler and lease logic
- worktree abstraction interfaces
- event and artifact types
- recovery logic
- deterministic fake-model test harness support

## 21.2 Golem responsibilities

Golem should own product behavior:

- CLI/TUI mission UX
- policy and config loading
- runtime wiring into existing agent execution
- worktree implementation against local git
- approval surfaces
- product-specific live smoke tests
- workflow panel integration using plan/invariants/verification views

## 21.3 Existing system alignment requirements

Mission execution must compose with existing workflow proofing:

- worker runs should surface task-local plan state
- worker and review runs should surface invariants when useful
- verification results should feed the existing verification UI concepts
- mission status should integrate with the current workflow panel rather than invent a second unrelated execution UI

## 21.4 Verified current Gollem API boundary

This repository currently depends on `github.com/fugue-labs/gollem` via `go.mod` with:

```go
replace github.com/fugue-labs/gollem => ../gollem
```

For this investigation, the local checkout available for that replace target was verified at `/Users/trevor/ws/gollem` on commit `adb6610b3d3792b4be28d2f1a0acfedf3907f8f9` (branch `codex/runtime-event-normalization`). That checkout also had uncommitted branch-local changes, so this section separates:

- **verified current API**: committed symbols present at `adb6610b3d3792b4be28d2f1a0acfedf3907f8f9`
- **proposed future/local-only API**: uncommitted additions visible only in that local checkout

### 21.4.1 Verified current reusable primitives in `ext/orchestrator`

The committed reusable orchestration surface is `ext/orchestrator`, not the speculative `ext/mission` package described earlier in this PRD.

**Verified current API at `adb6610b3d3792b4be28d2f1a0acfedf3907f8f9`:**

- task primitives: `orchestrator.Task`, `TaskStatus`, `RunRef`, `Lease`, `TaskResult`
- task mutation/query types: `CreateTaskRequest`, `UpdateTaskRequest`, `TaskFilter`, `ClaimTaskRequest`, `ClaimedTask`
- stores: `TaskStore`, `LeaseStore`, `CommandStore`, `ArtifactStore`
- command primitives: `Command`, `CommandKind`, `CommandStatus`, `CreateCommandRequest`, `CommandFilter`, `ClaimCommandRequest`
- artifact primitives: `Artifact`, `ArtifactSpec`, `CreateArtifactRequest`, `ArtifactFilter`
- runner/scheduler primitives: `TaskOutcome`, `Runner`, `RunnerFunc`, `AgentRunner`, `NewAgentRunner`, `WithTaskPrompt`, `WithTaskRunOptions`, `WithTaskResultMetadata`, `WithTaskArtifacts`, `Scheduler`, `NewScheduler`, `DefaultSchedulerConfig`, `WithWorkerID`, `WithPollInterval`, `WithLeaseTTL`, `WithLeaseRenewInterval`, `WithMaxConcurrentRuns`, `WithSchedulerClock`, `WithSchedulerErrorHandler`
- concrete lifecycle event structs published by the SQLite store through `core.EventBus`: `TaskCreatedEvent`, `TaskUpdatedEvent`, `TaskDeletedEvent`, `TaskClaimedEvent`, `LeaseRenewedEvent`, `LeaseReleasedEvent`, `TaskRequeuedEvent`, `TaskCompletedEvent`, `TaskFailedEvent`, `TaskCanceledEvent`, `ArtifactCreatedEvent`, `CommandCreatedEvent`, `CommandHandledEvent`

### 21.4.2 Verified current runtime events in `core`

The committed runtime event surface is in `core/runtime_events.go`:

- `core.RuntimeEvent`
- `core.RunStartedEvent`
- `core.RunCompletedEvent`
- `core.ToolCalledEvent`
- constructors `NewRunStartedEvent`, `NewRunCompletedEvent`, `NewToolCalledEvent`

These are the only verified current normalized runtime lifecycle events in this Gollem revision.

Important correction: `core.NormalizeHistory()` in `core/normalize.go` is **not** an orchestration-event normalizer. It only cleans `[]ModelMessage` history by removing orphaned tool returns, stripping stale images, and dropping empty requests. Any future runtime-event normalization layer must be introduced as a new API; it must not be documented as if `NormalizeHistory()` already does that job.

### 21.4.3 Proposed future/local-only API in the inspected checkout

The inspected local Gollem checkout also contained uncommitted additions that are **not** part of the verified current API at `adb6610b3d3792b4be28d2f1a0acfedf3907f8f9`:

- `ext/orchestrator/history.go` adding `EventKind`, `EventRecord`, `EventFilter`, and `EventStore`
- `ext/orchestrator/sqlite/history.go` adding SQLite persistence/query support for durable orchestration history
- corresponding uncommitted edits in `ext/orchestrator/types.go` / `ext/orchestrator/sqlite/store.go`

Those additions are useful directionally, especially for replacing Golem's local append-only mission event table, but they should be treated as **proposed future API on a local branch** until they are committed upstream and referenced by this repository.

## 21.5 Concrete mapping from current `internal/mission` to verified current Gollem primitives

The alignment boundary should be drawn around reusable execution/orchestration mechanics, while mission product semantics remain in Golem until Gollem grows explicit first-class support.

| Current Golem concern | Current source | Verified current Gollem primitive | Boundary decision |
|---|---|---|---|
| Mission identity, goal, repo root, base commit/branch, budget, success criteria, mission lifecycle | `internal/mission.Mission` | No committed `ext/orchestrator` equivalent | Keep product-local in Golem for now |
| Task durable identity | `internal/mission.Task.ID` | `orchestrator.Task.ID` | Align directly |
| Task type/category | `internal/mission.Task.Kind` | `orchestrator.Task.Kind` | Align directly |
| Task title | `internal/mission.Task.Title` | `orchestrator.Task.Subject` | Map directly |
| Task detailed objective | `internal/mission.Task.Objective` | `orchestrator.Task.Description` | Map directly |
| Task execution input/prompt seed | worker/reviewer prompt builders | `orchestrator.Task.Input` | Store normalized task run input here |
| Task dependencies | `TaskDependency`, `Controller.resolveReadyTasks`, `Recovery.resolveReadyTasks` | `orchestrator.Task.Blocks` / `BlockedBy` | Align directly; dependency edges should stop living in a parallel bespoke model |
| Task attempt accounting | `Task.AttemptCount` | `orchestrator.Task.Attempt`, `MaxAttempts`, `LastError`, `Retryable` | Align directly |
| Task priority, writable/readable scope, acceptance criteria, review requirements, estimated effort, risk level | `internal/mission.Task` fields | `orchestrator.Task.Metadata` | Keep semantics product-local, but persist inside canonical orchestrator metadata rather than separate task schema fields |
| Task ready state | `TaskReady` plus `GetReadyTasks` | derived from `orchestrator.TaskPending` plus dependency/lease checks in `TaskStore.ClaimReadyTask` | Make `ready` a derived product view, not a separate canonical state |
| Task running/leased state | `TaskLeased`, `TaskRunning`, `Run.LeaseOwner`, `Run.LeaseExpires`, `Run.HeartbeatAt` | `orchestrator.TaskRunning`, `Lease`, `LeaseStore.RenewLease`, `LeaseStore.ReleaseLease`, `RunRef` | Move canonical leasing into Gollem |
| Task awaiting review / accepted / integrated / done / rejected | `TaskAwaitingReview`, `TaskAccepted`, `TaskIntegrated`, `TaskDone`, `TaskRejected` | No committed first-class equivalents in `orchestrator.TaskStatus` | Keep as Golem product-level phase projection for now, backed by artifacts/metadata/approvals instead of a second lease model |
| Worker/review/integration run identity | `internal/mission.Run` | `orchestrator.RunRef` for the active attempt | Align attempt identity, but keep phase-specific run history/product reporting local until Gollem exposes a richer run model |
| Worker result summary, tool state, usage, structured output | `Run.Summary`, `Run.ErrorText`, worker artifacts | `orchestrator.TaskResult`, `TaskOutcome` | Align directly |
| Review result payload | `ReviewResult` | `TaskResult.Output` and/or `ArtifactStore` body | Reuse Gollem result/artifact primitives; keep review policy local |
| Integration result payload | `IntegrationResult` | `TaskResult.Output` and/or `ArtifactStore` body | Reuse Gollem artifact/result containers; keep integration policy local |
| Artifacts | `internal/mission.Artifact` | `orchestrator.Artifact`, `ArtifactSpec`, `ArtifactStore` | Align storage primitive; map `Type -> Kind`, `RelativePath/sha256 -> Name/Metadata` |
| Cancel/retry control flow | ad hoc controller/orchestrator transitions | `CommandCancelTask`, `CommandRetryTask`, `CommandStore` | Align task-scoped control commands to Gollem |
| Mission pause/resume/approve gates | `Controller.PauseMission`, approvals, dashboard state | No committed mission-level command/approval primitive | Keep product-local in Golem |
| Append-only mission event log | `internal/mission.Event`, `Store.AppendEvent` | committed event structs exist, but no committed durable `EventStore` at this revision | Keep durable event log local until upstream history API is committed |
| Runtime execution lifecycle | ad hoc run/orchestrator events | `core.RuntimeEvent`, `RunStartedEvent`, `RunCompletedEvent`, `ToolCalledEvent` | Align directly for agent/tool observability |

### 21.5.1 Status translation rule

The most important anti-divergence rule is that Golem should stop treating `ready`, `leased`, and `running` as a separate homegrown orchestration state machine once `ext/orchestrator` is adopted.

For the current committed Gollem API:

- **canonical schedulable states** should come from `orchestrator.TaskStatus` + dependency edges + lease presence
- **product-facing mission phases** such as `awaiting_review`, `accepted`, `integrated`, and `done` may remain Golem projections until Gollem grows explicit first-class support for post-run review/integration phases

That preserves one reusable execution model while still letting Golem render richer product UX.

### 21.5.2 Runtime-event alignment rule

Runtime-event alignment should use the committed `core` event surface exactly as it exists today:

- planner/worker/reviewer/integration agent start -> `core.RunStartedEvent`
- planner/worker/reviewer/integration agent finish -> `core.RunCompletedEvent`
- tool invocation within those runs -> `core.ToolCalledEvent`

Mission-specific events such as `mission.created`, `plan.applied`, `review.passed`, `integration.completed`, or approval resolution remain product-level orchestration events. They should not be mislabeled as `core.RuntimeEvent` unless Gollem later introduces a committed broader event taxonomy.

## 21.6 Migration sequence

The order of operations matters. Persistence and execution should align first; CLI/TUI should remain adapters over derived state until the lower-level boundary is stable.

### 21.6.1 Change first: persistence and execution boundary

1. **Adopt `ext/orchestrator` for task execution state before changing UI flows.**
   - Replace Golem-local ready/lease bookkeeping in `internal/mission/scheduler.go`, `worker.go`, and recovery logic with `TaskStore`, `LeaseStore`, `CommandStore`, and `Scheduler`.
   - Keep mission rows and approval rows local while task execution moves to Gollem primitives.
2. **Move worker results and evidence to `TaskResult` + `ArtifactStore`.**
   - Treat current worker summary, verification evidence, diffs, and review JSON as canonical result/artifact payloads rather than bespoke run-table fields.
3. **Emit committed `core.RuntimeEvent` values for all agent/tool lifecycle telemetry.**
   - Join them back to mission/task IDs in Golem projection code as needed.
4. **Use `CommandStore` for task-scoped cancel/retry.**
   - Stop inventing one-off task retry/cancel control paths in Golem once Gollem commands are available.

### 21.6.2 Change second: post-run mission phases

After the execution core is aligned, choose one of these paths for review/integration phases:

1. **Short-term product-local projection**
   - keep `awaiting_review` / `accepted` / `integrated` / `done` as Golem-derived mission/task views backed by orchestrator task metadata, review artifacts, and approval records
2. **Future upstream extension**
   - if review/integration semantics need to be reusable across products, upstream either:
     - richer task phase/status support in `ext/orchestrator`, or
     - a thin mission wrapper package on top of `ext/orchestrator`

Do **not** fork a second lease/scheduler model in Golem while waiting for that future extension.

### 21.6.3 Safe to keep product-specific in Golem

These concerns can remain product-local without creating orchestration drift:

- `internal/ui/mission_commands.go` command parsing and help text
- `internal/ui/dashboard/dashboard.go` rendering, focus, and summary panes
- `main.go` dashboard entrypoint and CLI wiring
- mission goal/policy/budget UX
- approval UX and human decision presentation
- local git/worktree shelling in `internal/mission/worktree.go`
- mission summary/task-count projections used only for display

## 21.7 Package dependency rules

Implementation must follow these dependency rules:

1. `internal/mission` in Golem may depend on verified Gollem primitives in `ext/orchestrator` and `core/runtime_events.go`, plus product config, local git/worktree code, and UI adapters.
2. Gollem orchestration packages must not depend on Bubble Tea UI code or Golem-specific mission rendering.
3. Store code must not import UI rendering types.
4. Planner/worker/review runners should depend on `orchestrator.Task`, `ClaimedTask`, `TaskOutcome`, `TaskResult`, and artifacts rather than bespoke UI models.
5. UI packages may observe derived mission state and runtime events, but must not own canonical lease/scheduler transitions.
6. Approval rendering code may format approval requests, but only Golem's mission controller may resolve product-local approval state until Gollem exposes a committed reusable approval primitive.

## 21.8 Anti-corruption boundary between Golem and Gollem

The product/framework seam must be explicit.

- **Gollem owns reusable execution mechanics**: task persistence, dependency edges, leases, task-scoped commands, artifacts, runner glue, scheduler logic, and runtime lifecycle events.
- **Golem owns product semantics**: missions, mission phases, approval UX, dashboard rendering, CLI/TUI wording, repo-specific worktree policy, and integration/review policy that is not yet first-class in Gollem.

Rules:

1. Golem may translate mission/task UX into `ext/orchestrator` requests, but must not fork canonical lease, claim, retry, or artifact semantics.
2. Golem-specific view models must be projections over canonical orchestrator state plus product-local mission state.
3. If a concern is reusable across products and already has a committed Gollem primitive, use that primitive instead of extending `internal/mission` independently.
4. If a concern is not yet covered by committed Gollem API, keep it explicitly product-local and document it as such rather than pretending Gollem already provides it.

Violations of this boundary are architecture regressions.

---

## 22. Package and File Layout

## 22.1 Verified current Gollem layout

```text
core/
  runtime_events.go
  normalize.go

ext/orchestrator/
  types.go
  store.go
  commands.go
  artifacts.go
  events.go
  runner.go
  scheduler.go
  memory/
  sqlite/
```

### File responsibilities

- `core/runtime_events.go`: committed runtime lifecycle event interface and concrete event types
- `core/normalize.go`: message-history cleanup only; not orchestration-event normalization
- `ext/orchestrator/types.go`: task, run-ref, lease, task-result, and request/filter primitives
- `ext/orchestrator/store.go`: `TaskStore` / `LeaseStore` interfaces
- `ext/orchestrator/commands.go`: task-scoped control-command primitives and `CommandStore`
- `ext/orchestrator/artifacts.go`: immutable task/run artifact primitives and `ArtifactStore`
- `ext/orchestrator/events.go`: concrete lifecycle event payload structs published via `core.EventBus`
- `ext/orchestrator/runner.go`: `Runner`, `TaskOutcome`, and `AgentRunner` glue to `core.Agent`
- `ext/orchestrator/scheduler.go`: reusable polling/lease-renewal/concurrency scheduler
- `ext/orchestrator/memory` and `ext/orchestrator/sqlite`: concrete store implementations

## 22.2 Proposed future Gollem additions (not verified current API)

Only after the API is committed upstream should this repository depend on any of the following:

- durable orchestration history APIs such as `EventRecord` / `EventStore`
- a higher-level `ext/mission` wrapper package
- first-class review/integration/approval mission semantics above `ext/orchestrator`
- runtime-event normalization beyond the currently committed `core.RuntimeEvent` types

## 22.3 Expected current Golem integration points

- `internal/mission/types.go`
- `internal/mission/store.go`
- `internal/mission/controller.go`
- `internal/mission/orchestrator.go`
- `internal/mission/scheduler.go`
- `internal/mission/worker.go`
- `internal/mission/reviewer.go`
- `internal/mission/integrator.go`
- `internal/mission/recovery.go`
- `internal/ui/mission_commands.go`
- `internal/ui/dashboard/dashboard.go`
- `main.go`

---

## 23. Testing Strategy

The mission system is incomplete until deterministic and live verification both exist.

## 23.1 Unit tests

Required unit coverage:

1. mission state transitions
2. task state transitions
3. scheduler selection rules
4. scope overlap detection
5. lease expiry and reclamation
6. structural vs local replan classification
7. artifact generation and validation
8. approval state transitions
9. recovery reconciliation logic
10. integration eligibility rules

## 23.2 Integration tests

Required integration coverage:

1. create -> plan -> start -> complete on a toy repo
2. worker fail -> retry/replan path
3. review rejection prevents integration
4. pause/resume with durable state reload
5. expired lease reclamation after simulated worker death
6. conflicting writable scopes are not scheduled together
7. accepted tasks integrate in dependency order
8. restart process and recover active mission correctly

These tests must use:

- temporary git repos
- real worktree creation
- on-disk SQLite store
- deterministic fake model runner

## 23.3 Golden tests

Required golden coverage:

- plan output for known mission prompts under fake model
- mission status JSON
- mission board render states
- artifact rendering helpers

### Required UI golden states

At minimum, the mission UI should have stable snapshot/golden coverage for:

1. draft mission with no tasks
2. planning mission with spinner/progress state
3. running mission with multiple active workers
4. blocked mission with no runnable tasks
5. review rejection surfaced in board + detail pane
6. pending approval surfaced in queue + approval detail
7. completed mission with final evidence visible
8. recovered mission after restart with lease-lost reconciliation shown

## 23.4 Invariant tests

Required invariants:

- no task becomes `done` without review pass and integration success
- no task is leased twice concurrently
- mission completion requires success criteria satisfaction
- integration never runs on rejected work
- required artifacts exist for every accepted task

## 23.5 Fault injection tests

Simulate:

- worker crash
- review crash
- SQLite write failure
- worktree creation failure
- partial artifact generation failure
- lease heartbeat loss
- integration conflict

Each fault must have an asserted recovery path.

## 23.6 Mission controller loop pseudocode

The mission controller loop should be simple enough to reason about directly.

Illustrative pseudocode:

```go
func (c *Controller) Tick(ctx context.Context, missionID string) error {
    mission := c.store.GetMission(ctx, missionID)

    if mission.IsTerminal() {
        return nil
    }

    if err := c.recoverExpiredLeases(ctx, mission); err != nil {
        return err
    }

    if approval := c.store.GetBlockingApproval(ctx, missionID); approval != nil {
        c.emitWaitingForApproval(mission, approval)
        return nil
    }

    if c.shouldReplan(ctx, mission) {
        proposal, err := c.planner.Replan(ctx, c.buildReplanRequest(mission))
        if err != nil {
            return c.recordPlannerFailure(ctx, mission, err)
        }
        return c.handleReplanProposal(ctx, mission, proposal)
    }

    accepted := c.store.ListAcceptedNotIntegratedTasks(ctx, missionID)
    for _, task := range accepted {
        result, err := c.integration.ApplyAcceptedTask(ctx, c.buildIntegrationRequest(mission, task))
        if err != nil {
            return c.recordIntegrationFailure(ctx, mission, task, err)
        }
        if err := c.applyIntegrationResult(ctx, mission, task, result); err != nil {
            return err
        }
    }

    for {
        claim, err := c.tasks.ClaimReadyTask(ctx, orchestrator.ClaimTaskRequest{
            WorkerID: c.workerID,
            LeaseTTL: c.leaseTTL,
            Now:      c.now(),
        })
        if errors.Is(err, orchestrator.ErrNoReadyTask) {
            break
        }
        if err != nil {
            return err
        }
        go c.runWorkerOrReviewer(c.childContext(missionID), claim)
    }

    return c.updateMissionPhase(ctx, missionID)
}
```

Required behavioral properties of the loop:

1. one tick must be safe to rerun after crash or restart
2. integration must happen only after review pass
3. blocking approval must halt new leases
4. replan proposal must not partially mutate the plan before decision
5. terminal missions must be no-op under further ticks

### Required controller subroutine pseudocode

The implementation should also spell out and unit-test these subroutines:

```go
func (c *Controller) shouldReplan(ctx context.Context, mission Mission) bool
func (c *Controller) handleReplanProposal(ctx context.Context, mission Mission, draft PlanResult) error
func (c *Controller) applyIntegrationResult(ctx context.Context, mission Mission, task Task, result IntegrationResult) error
func (c *Controller) recoverExpiredLeases(ctx context.Context, mission Mission) error
```

Behavioral intent:

- `shouldReplan` evaluates durable facts only
- `handleReplanProposal` is responsible for approval-vs-auto-apply routing
- `applyIntegrationResult` performs canonical task/mission transitions and follow-up task creation
- `recoverExpiredLeases` reconciles dead runs without double-leasing work

## 23.7 Performance tests

Required performance checks:

- scheduler remains reasonable with at least 1,000 queued tasks
- event replay does not cause unbounded memory growth
- TUI remains responsive under high event throughput

## 23.8 Evaluation fixtures

Mission evaluation requires dedicated tiny repos and scenario fixtures.

Recommended fixture families:

1. **planning_only_repo**
   - enough structure to test decomposition quality without code edits
2. **parallel_scopes_repo**
   - two or more clearly independent writable scopes
3. **review_rejection_repo**
   - encourages believable worker overclaim or incomplete fix
4. **repair_loop_repo**
   - requires at least one failed attempt and one successful follow-up
5. **conflict_boundary_repo**
   - tasks that look parallel but actually share writable scope risk
6. **pause_resume_repo**
   - long enough execution path to test persistence and recovery

Fixture quality requirements:

- tiny and cheap to run
- deterministic build/test scripts
- readable enough for maintainers to understand failure causes
- deliberately shaped to exercise one primary orchestration behavior each

## 23.8.1 Concrete fixture guidance

### planning_only_repo
Must contain:
- at least 3 subsystems or directories
- a real but small dependency graph the planner can reason about
- no required file edits for a planning-only success path

### parallel_scopes_repo
Must contain:
- two failing or incomplete areas in disjoint paths
- deterministic checks per area
- obvious opportunity for safe parallelism

### review_rejection_repo
Must contain:
- a bug that can be superficially "fixed" while still violating an acceptance criterion
- deterministic tests or checks that expose the overclaim

### repair_loop_repo
Must contain:
- an initial task likely to fail review or integration on the first attempt
- a clear follow-up path that can succeed on the second attempt

### conflict_boundary_repo
Must contain:
- two tasks whose scopes appear independent at first glance
- a shared integration hotspot to exercise conflict detection/replanning

### pause_resume_repo
Must contain:
- enough tasks and timing to exercise mid-mission persistence
- deterministic signals showing whether resume correctly preserved progress

## 23.8.2 Fixture directory recommendation

Recommended structure:

```text
internal/mission/testdata/
  planning_only_repo/
  parallel_scopes_repo/
  review_rejection_repo/
  repair_loop_repo/
  conflict_boundary_repo/
  pause_resume_repo/
```

Each fixture should include a short README describing:
- intended orchestration behavior
- expected acceptance checks
- common failure modes

## 23.9 Evaluation scoring heuristics

Deterministic evaluation should score at least:

- planning boundedness
- task scope quality
- review correctness
- integration correctness
- recovery correctness
- operator explainability of the resulting mission state

A mission system that technically completes tasks but produces unreadable plans, noisy approvals, or opaque recovery behavior is not meeting the product bar.

## 23.10 Live evaluation discipline

Live smoke scenarios should be written so that failure is informative.

That means each live scenario should isolate one primary orchestration claim:

- planning quality
- safe parallel execution
- review catches overclaim
- repair loop works
- pause/resume works

Do not create giant end-to-end live scenarios that fail ambiguously.

---

## 24. Live LLM Smoke Testing

Live smoke tests are required for ship confidence but remain opt-in during normal local development.

## 24.1 Goals

Prove the real mission workflow works with actual provider calls, not only fake models.

## 24.2 Gate

Suggested gate:

```bash
GOLEM_LIVE_SMOKE=1 go test -tags=live ./internal/mission -run Live
```

## 24.3 Required scenarios

### Scenario A: planning-only mission
Must verify:
- bounded task graph generation
- approval flow
- no illegal writes

### Scenario B: two independent parallel tasks
Must verify:
- scheduler leases both safely
- worker scopes remain independent
- both tasks review and integrate

### Scenario C: review catches unsupported claim
Must verify:
- worker overclaims
- review rejects
- task does not integrate
- mission requeues or replans correctly

### Scenario D: repair loop
Must verify:
- initial attempt fails review
- follow-up task is created
- later attempt succeeds

### Scenario E: pause/resume mid-mission
Must verify:
- mission pauses correctly
- restart resumes from durable truth

## 24.4 Provider matrix

Minimum ship bar:

1. primary production provider passes the full suite
2. one secondary provider passes at least a reduced suite

## 24.5 Evidence retention

Every live smoke run must retain:

- mission snapshot
- task artifacts
- commands run
- final diff
- review evidence
- final integration evidence

A live smoke run is incomplete if evidence retention fails.

---

## 25. Acceptance Criteria

The feature is accepted only when all of the following are true:

1. A user can create, plan, start, pause, resume, and cancel a mission.
2. A mission can run multiple workers concurrently toward a shared goal.
3. Writable scope conflicts are prevented or explicitly policy-approved.
4. Every accepted task has a task spec, run summary, diff or no-op proof, verification evidence, and review result.
5. No task integrates without review pass.
6. Crash/restart recovery preserves correctness.
7. The TUI exposes enough state for an operator to understand progress, blockage, and pending approvals.
8. Deterministic tests cover lifecycle, conflict handling, and recovery.
9. Live smoke tests prove the end-to-end workflow with real provider calls.
10. The final mission outcome is backed by a durable, auditable evidence trail.

---

## 26. Implementation Order

Implementation must follow this order.

## 26.1 Migration and backfill strategy

The mission system is a new subsystem, but the implementation should still treat schema evolution seriously from day one.

Requirements:

1. store schema must be versioned
2. startup must run idempotent migrations before opening mission services
3. the store must fail fast on unknown future schema versions
4. derived data must be recomputable from canonical rows plus artifacts
5. no migration may require in-memory-only backfill logic

Recommended approach:

- maintain a schema version table
- keep SQL migrations in ordered files
- prefer additive migrations in early versions
- treat artifacts as immutable once recorded except for deterministic regeneration of derived artifacts

Backfill policy:

- if a new derived field is introduced later, recompute it from canonical state/events/artifacts on startup or first access
- do not rewrite old event history unless there is a compelling corruption fix
- when backfilling derived mission summaries, emit a backfill event for auditability

## 26.2 Package-by-package implementation sequence

### Phase 1: core store and state

- state types
- SQLite schema and migrations
- event log
- state transition helpers
- artifact persistence helpers

Definition of done:
- schema boots from empty state
- state transitions are unit tested
- event append and artifact recording are durable

### Phase 2: controller and scheduler

- mission controller
- scheduler
- lease logic
- recovery logic

Definition of done:
- controller can create, load, tick, and recover missions deterministically
- scheduler honors scope conflicts and review capacity

### Phase 3: worker and review execution

- worktree manager
- worker execution mode
- review execution mode
- derived artifact generation

Definition of done:
- one task can execute end-to-end through review with durable artifacts

### Phase 4: integration and approvals

- integration engine
- post-apply validation
- approval lifecycle

Definition of done:
- accepted work integrates only through the integration engine
- blocking approvals survive restart and resolve deterministically

### Phase 5: product surface

- CLI commands
- TUI mission board
- workflow panel integration
- status JSON

Definition of done:
- operator can drive and inspect a mission entirely from CLI/TUI

### Phase 6: verification

- unit tests
- integration tests
- fault injection tests
- live smoke tests

No phase may be declared complete without its associated tests.

---

## 27. Risks and Mitigations

### Risk: uncontrolled task explosion
Mitigation:
- planner caps task count per mission
- structural replans require approval by default

### Risk: scope conflicts still slip through
Mitigation:
- scheduler conflict exclusion
- integration conflict detection
- narrow writable scopes by default

### Risk: review is too weak
Mitigation:
- prioritize deterministic checks
- reject unsupported claims aggressively
- require evidence-rich review output

### Risk: mission feels overbuilt
Mitigation:
- keep v1 single-process
- reuse existing runtime surfaces
- keep integration deterministic by default

### Risk: live tests become flaky
Mitigation:
- tiny fixture repos
- low-variance prompts
- strict evidence retention
- no silent tolerance for semantic failures

---

## 28. Anti-patterns

The implementation must not:

1. Turn mission orchestration into a freeform agent-to-agent chat swarm.
2. Introduce a daemon or remote protocol as a v1 requirement.
3. Merge worker changes without an independent review result.
4. Make artifact generation depend entirely on model obedience when deterministic generation is possible.
5. Add approval prompts that do not correspond to meaningful risk.
6. Create separate, conflicting notions of mission state across UI, runtime, and store.
7. Claim the feature is complete without deterministic tests and live smoke evidence.

---

## 29. Final Notes to the Implementing Agent

1. Build the simplest system that satisfies the behavioral requirements.
2. Favor explicit state, explicit evidence, and deterministic transitions.
3. If a design choice makes the system feel more distributed than necessary, more magical than auditable, or more complicated than explainable, that design choice is wrong.
4. If a decision belongs in the reusable orchestration core, put it in Gollem.
5. If a decision belongs to product UX or presentation, keep it in Golem.

The correct v1 mission system should feel like a natural extension of Golem’s existing disciplined coding workflow, not like a separate platform stapled onto it.

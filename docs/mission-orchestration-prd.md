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
   - The user can pause, resume, approve, reject, retry, and cancel.

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
3. As a developer, I can require approval before risky escalations or final integration.
4. As a developer, I can pause a mission, restart Golem, and continue later.
5. As a developer, I can inspect why a task failed review and decide whether to retry, replan, or stop.
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

A mission visibly moves through these phases:

1. **Draft**
   - goal captured, not yet planned
2. **Planning**
   - controller is building or revising the task graph
3. **Awaiting approval**
   - human decision required
4. **Running**
   - workers and reviewers are active
5. **Blocked**
   - no runnable tasks and no safe automatic progress path
6. **Paused**
   - operator paused mission
7. **Completing**
   - final integration / final checks in progress
8. **Completed / Failed / Cancelled**

---

## 8. Scope Boundary

### 8.1 In scope for v1

- local-first, single-host orchestration
- multiple concurrent worker runs in separate git worktrees
- independent review runs
- durable mission/task/run/artifact/event storage
- mission create / plan / start / pause / resume / cancel
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
- request planning and replanning
- schedule runnable tasks
- lease and monitor runs
- collect artifacts
- request approvals when policy requires them
- trigger review
- integrate reviewed work
- emit events for UI and logs
- perform recovery on startup

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

Recommended event families:

### Mission lifecycle events
- `mission_created`
- `mission_planning_started`
- `mission_planning_completed`
- `mission_started`
- `mission_paused`
- `mission_resumed`
- `mission_blocked`
- `mission_completing`
- `mission_completed`
- `mission_failed`
- `mission_cancelled`

### Plan and replan events
- `replan_requested`
- `replan_proposed`
- `replan_approved`
- `replan_rejected`
- `replan_applied`
- `replan_superseded`

### Task lifecycle events
- `task_created`
- `task_ready`
- `task_blocked`
- `task_leased`
- `task_requeued`
- `task_awaiting_review`
- `task_accepted`
- `task_rejected`
- `task_integrated`
- `task_done`
- `task_superseded`

### Run lifecycle events
- `run_created`
- `run_started`
- `run_heartbeat`
- `run_succeeded`
- `run_failed`
- `run_cancelled`
- `run_timed_out`
- `run_lease_lost`

### Approval events
- `approval_created`
- `approval_superseded`
- `approval_approved`
- `approval_rejected`
- `approval_expired`

### Artifact events
- `artifact_recorded`
- `artifact_missing`
- `artifact_validation_failed`

### Recovery and integration events
- `recovery_started`
- `recovery_reconciled_run`
- `recovery_completed`
- `integration_started`
- `integration_succeeded`
- `integration_failed`

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

By default, approvals are required for:

- initial plan when mission is not explicitly auto-started
- structural replan
- writable scope escalation beyond the task spec
- high-risk command escalation
- integration into the user-facing branch/worktree when policy requires it
- final mission completion when policy requires sign-off

## 18.4 Approval lifecycle semantics

Every approval request must have:

- a stable `approval_id`
- `kind`
- `status` in `pending|approved|rejected|superseded|expired`
- `mission_id`
- optional `task_id` and `run_id`
- `decision_surface_json`
- `requested_at`
- `resolved_at`
- optional `superseded_by`

Controller rules:

1. there must be at most one active blocking approval for the same decision surface
2. repeated identical approval requests must coalesce instead of spamming the operator
3. a new approval that makes an older one obsolete must mark the older request `superseded`
4. approvals must be durable across restart
5. rejecting an approval must always produce a deterministic next state:
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

## 20.1 CLI commands

Required CLI surface:

```bash
golem mission create --goal "..." [--title "..."]
golem mission plan <mission-id>
golem mission replan <mission-id> [--reason "..."]
golem mission start <mission-id>
golem mission status <mission-id> [--json]
golem mission pause <mission-id> [--hard]
golem mission resume <mission-id>
golem mission cancel <mission-id>
golem mission tasks <mission-id> [--json]
golem mission task show <task-id> [--json]
golem mission task retry <task-id>
golem mission runs <mission-id> [--json]
golem mission run show <run-id> [--json]
golem mission approvals <mission-id> [--json]
golem mission approval show <approval-id> [--json]
golem mission approve <approval-id>
golem mission reject <approval-id> --reason "..."
golem mission replay <mission-id>
golem mission artifacts <task-id|run-id>
```

## 20.1.1 CLI JSON output contracts

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

The TUI must provide:

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

From the TUI, the user must be able to:

- create a mission
- approve/reject the plan
- inspect task details and artifacts
- pause/resume/cancel
- retry a task
- force a replan
- approve/reject escalation
- inspect final completion evidence

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

## 21.4 Core Go interface contracts

The design should use **interfaces at true subsystem boundaries** and keep concrete orchestration types concrete.

Required interface boundaries:

```go
type MissionStore interface {
    CreateMission(ctx context.Context, m Mission) (Mission, error)
    GetMission(ctx context.Context, missionID string) (Mission, error)
    SavePlan(ctx context.Context, missionID string, plan PlanSnapshot) error
    LeaseReadyTasks(ctx context.Context, missionID string, req LeaseRequest) ([]TaskLease, error)
    HeartbeatRun(ctx context.Context, runID string, at time.Time) error
    CompleteRun(ctx context.Context, result RunResult) error
    CreateApproval(ctx context.Context, req ApprovalRequest) (Approval, error)
    ResolveApproval(ctx context.Context, decision ApprovalDecision) error
    AppendEvent(ctx context.Context, evt Event) error
    RecordArtifact(ctx context.Context, a ArtifactRecord) error
}

type Planner interface {
    InitialPlan(ctx context.Context, req InitialPlanRequest) (PlanDraft, error)
    Replan(ctx context.Context, req ReplanRequest) (PlanDraft, error)
}

type RoleRunner interface {
    Run(ctx context.Context, spec RunSpec) (RunResult, error)
}

type IntegrationEngine interface {
    ApplyAcceptedTask(ctx context.Context, req IntegrationRequest) (IntegrationResult, error)
}

type WorktreeManager interface {
    Create(ctx context.Context, spec WorktreeSpec) (WorktreeHandle, error)
    Cleanup(ctx context.Context, handle WorktreeHandle) error
}
```

## 21.4.1 Core Go data types

The implementation should use explicit typed models rather than map-heavy orchestration code.

Illustrative shapes:

```go
type Mission struct {
    ID              string
    Title           string
    Goal            string
    RepoRoot        string
    BaseCommit      string
    BaseBranch      string
    Status          MissionStatus
    Policy          MissionPolicy
    Budget          MissionBudget
    SuccessCriteria []string
    IntegrationRef  string
    CreatedAt       time.Time
    UpdatedAt       time.Time
    StartedAt       *time.Time
    EndedAt         *time.Time
    LastReplanAt    *time.Time
}

type Task struct {
    ID                 string
    MissionID          string
    Title              string
    Kind               TaskKind
    Objective          string
    Status             TaskStatus
    Priority           int
    Scope              TaskScope
    AcceptanceCriteria []string
    ReviewRequirements []string
    EstimatedEffort    string
    RiskLevel          RiskLevel
    AttemptCount       int
    BlockingReason     string
    CreatedAt          time.Time
    UpdatedAt          time.Time
}

type Run struct {
    ID             string
    MissionID      string
    TaskID         string
    Mode           RunMode
    Status         RunStatus
    LeaseOwner     string
    LeaseExpiresAt *time.Time
    HeartbeatAt    *time.Time
    WorktreePath   string
    StartedAt      *time.Time
    EndedAt        *time.Time
    Summary        string
    ErrorText      string
}

type Approval struct {
    ID           string
    MissionID    string
    TaskID       string
    RunID        string
    Kind         ApprovalKind
    Status       ApprovalStatus
    RequestJSON  []byte
    ResponseJSON []byte
    CreatedAt    time.Time
    ResolvedAt   *time.Time
}

type Event struct {
    ID        int64
    MissionID string
    TaskID    string
    RunID     string
    Type      EventType
    Payload   []byte
    CreatedAt time.Time
}
```

These types should live close to the mission primitives, not be spread across UI packages.

## 21.4.2 Controller-facing request/result types

The orchestration core should use typed request/result structs for all boundary crossings.

Minimum expected shapes:

```go
type LeaseRequest struct {
    MaxLeases        int
    ReserveReviewers int
    Now              time.Time
}

type TaskLease struct {
    Task          Task
    Run           Run
    LeaseToken    string
    LeaseDeadline time.Time
}

type RunSpec struct {
    Mission   Mission
    Task      Task
    Run       Run
    Worktree  WorktreeHandle
    TaskSpec  TaskSpecArtifact
    Policy    MissionPolicy
}

type RunResult struct {
    RunID         string
    Status        RunStatus
    Summary       string
    ErrorText     string
    ArtifactRefs  []ArtifactRecord
    FollowupTasks []TaskDraft
}

type IntegrationRequest struct {
    Mission Mission
    Task    Task
    Run     Run
}

type IntegrationResult struct {
    Result        string
    AppliedCommit string
    ArtifactRefs  []ArtifactRecord
    FollowupTasks []TaskDraft
}
```

Using typed request/result structs is required to keep the controller loop understandable and testable.

v1 should avoid interfaces for:

- the mission controller itself
- the scheduler core
- pure state transition helpers

Those should be concrete packages/types unless a second real implementation appears.

## 21.4.3 Near-code mission controller API

The implementation should make the controller API explicit enough that a coding agent can scaffold it directly.

Illustrative shape:

```go
type Controller struct {
    store       MissionStore
    planner     Planner
    runner      RoleRunner
    integration IntegrationEngine
    worktrees   WorktreeManager
    clock       Clock
    logger      Logger
}

func NewController(deps ControllerDeps) (*Controller, error)
func (c *Controller) CreateMission(ctx context.Context, req CreateMissionRequest) (Mission, error)
func (c *Controller) StartMission(ctx context.Context, missionID string) error
func (c *Controller) PauseMission(ctx context.Context, missionID string, hard bool) error
func (c *Controller) ResumeMission(ctx context.Context, missionID string) error
func (c *Controller) CancelMission(ctx context.Context, missionID string) error
func (c *Controller) Tick(ctx context.Context, missionID string) error
func (c *Controller) Recover(ctx context.Context, missionID string) error
func (c *Controller) Approve(ctx context.Context, approvalID string) error
func (c *Controller) Reject(ctx context.Context, approvalID string, reason string) error
```

Behavioral expectations:

1. `CreateMission` persists canonical mission state before any planning work begins
2. `StartMission` only transitions from an allowed pre-run state
3. `Tick` is the single orchestration step function and must stay idempotent enough for crash/restart tolerance
4. `Recover` must reconcile leases, worktrees, and derived mission phase
5. approval methods must resolve canonical approval state and emit matching events

## 21.5 Package dependency rules

Implementation must follow these dependency rules:

1. `internal/mission` in Golem may depend on Gollem mission primitives, config, local git/worktree code, and UI adapters.
2. Gollem mission packages must not depend on Bubble Tea UI code.
3. Store code must not import UI rendering types.
4. Planner/worker/review runners must depend on abstract run specs and result types, not direct UI models.
5. UI packages may observe mission state and events, but must not own canonical mission transitions.
6. Approval rendering code may format approval requests, but only the mission controller may resolve canonical approval state.

## 21.6 Anti-corruption boundary between Golem and Gollem

The product/framework seam must be explicit.

Gollem mission packages are the reusable orchestration core.
Golem mission packages are the product adapter layer.

Rules:

1. Gollem must not import Bubble Tea, product styling, or CLI presentation code.
2. Golem may translate product config/UI actions into Gollem mission requests, but must not fork canonical state semantics.
3. JSON/CLI formatting belongs in Golem.
4. canonical mission state transitions, event taxonomy, and artifact contracts belong in Gollem mission primitives.
5. any convenience view model introduced in Golem must be derived from canonical mission state, not become a second source of truth.

Violations of this boundary are architecture regressions.

---

## 22. Package and File Layout

## 22.1 Recommended Gollem layout

```text
ext/mission/
  mission.go
  types.go
  store.go
  sqlite_store.go
  scheduler.go
  controller.go
  planner.go
  runner.go
  review.go
  integration.go
  artifacts.go
  events.go
  policy.go
  worktree.go
  recovery.go
  fake_model.go
```

### File responsibilities

- `mission.go`: public entry points and high-level package docs
- `types.go`: core enums, data types, and JSON-serializable contracts
- `store.go`: MissionStore interface and transaction boundary helpers
- `sqlite_store.go`: concrete SQLite persistence and migration wiring
- `scheduler.go`: ready-queue selection, scope conflict rules, lease assignment
- `controller.go`: orchestration entry points and controller subroutines
- `planner.go`: planner interface and planner request/result shaping
- `runner.go`: shared role-runner orchestration glue
- `review.go`: review result normalization and review-specific helpers
- `integration.go`: integration engine contracts and deterministic apply helpers
- `artifacts.go`: artifact naming, validation, and hashing helpers
- `events.go`: event taxonomy, event payload structs, and append helpers
- `policy.go`: budget checks, approval classification, retry policy helpers
- `worktree.go`: worktree abstractions independent of concrete git shelling
- `recovery.go`: startup reconciliation and lease-loss recovery helpers
- `fake_model.go`: deterministic fixture-model harness for tests

## 22.2 Recommended Golem layout

```text
internal/mission/
  service.go
  commands.go
  runtime.go
  approval.go
  worktree.go
  smoke_live_test.go

internal/ui/
  mission_model.go
  mission_view.go
  mission_cmds.go
```

### File responsibilities

- `internal/mission/service.go`: product-facing mission service wiring to Gollem primitives
- `internal/mission/commands.go`: CLI command registration and JSON output shaping
- `internal/mission/runtime.go`: provider/runtime configuration for planner/worker/reviewer runs
- `internal/mission/approval.go`: approval presentation helpers and policy-driven decision mapping
- `internal/mission/worktree.go`: local git worktree implementation
- `internal/mission/smoke_live_test.go`: opt-in live-provider smoke scenarios
- `internal/ui/mission_model.go`: Bubble Tea mission state model and message handling
- `internal/ui/mission_view.go`: mission board rendering and detail panes
- `internal/ui/mission_cmds.go`: async commands bridging UI actions to mission service operations

## 22.3 Expected current integration points

- `main.go`
- `internal/config/config.go`
- `internal/agent/agent.go`
- `internal/agent/runtime_state.go`
- `internal/ui/app.go`
- `internal/ui/commands.go`
- `internal/ui/workflow_panel.go`

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

    ready := c.store.ListReadyTasks(ctx, missionID)
    leases, err := c.store.LeaseReadyTasks(ctx, missionID, c.buildLeaseRequest(mission, ready))
    if err != nil {
        return err
    }

    for _, lease := range leases {
        go c.runWorkerOrReviewer(c.childContext(missionID), lease)
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
func (c *Controller) handleReplanProposal(ctx context.Context, mission Mission, draft PlanDraft) error
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

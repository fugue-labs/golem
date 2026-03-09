# Mission Orchestration PRD

## 1. Status

- **Product**: Golem (terminal product) + Gollem (agent framework)
- **Feature name**: Mission Orchestration / Mission Control
- **Document purpose**: implementation-ready product requirements for a state-of-the-art multi-agent mission system
- **Scope**: v1 local-first, single-host mission orchestration for repository work
- **Audience**: implementation agents, maintainers, reviewers
- **Quality bar**: the system must feel competitive with the best terminal coding agents in reliability, sophistication, and operator trust while remaining materially simpler in architecture than a daemon-first platform
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
  last_replan_at TIMESTAMP
);
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
  FOREIGN KEY (mission_id) REFERENCES missions(id)
);
```

### `task_dependencies`

```sql
CREATE TABLE task_dependencies (
  task_id TEXT NOT NULL,
  depends_on_task_id TEXT NOT NULL,
  PRIMARY KEY (task_id, depends_on_task_id)
);
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
  FOREIGN KEY (mission_id) REFERENCES missions(id),
  FOREIGN KEY (task_id) REFERENCES tasks(id)
);
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
  created_at TIMESTAMP NOT NULL
);
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
  resolved_at TIMESTAMP
);
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
  created_at TIMESTAMP NOT NULL
);
```

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

## 16.2 Artifact generation requirements

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

---

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

## 19.3 Review contract

A review run must:

- treat worker output as untrusted
- validate claims against artifacts and repository state
- reject unsupported claims
- produce explicit pass/fail evidence
- recommend next action when rejecting

## 19.4 Planning contract

A planning run must:

- create bounded tasks
- minimize writable scope overlap
- avoid unnecessary task explosion
- separate execution from review concerns
- explain any structural replan

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

v1 should avoid interfaces for:

- the mission controller itself
- the scheduler core
- pure state transition helpers

Those should be concrete packages/types unless a second real implementation appears.

## 21.5 Package dependency rules

Implementation must follow these dependency rules:

1. `internal/mission` in Golem may depend on Gollem mission primitives, config, local git/worktree code, and UI adapters.
2. Gollem mission packages must not depend on Bubble Tea UI code.
3. Store code must not import UI rendering types.
4. Planner/worker/review runners must depend on abstract run specs and result types, not direct UI models.
5. UI packages may observe mission state and events, but must not own canonical mission transitions.
6. Approval rendering code may format approval requests, but only the mission controller may resolve canonical approval state.

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

## 23.6 Performance tests

Required performance checks:

- scheduler remains reasonable with at least 1,000 queued tasks
- event replay does not cause unbounded memory growth
- TUI remains responsive under high event throughput

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
- review reports
- completion summary

A smoke run is incomplete if evidence retention fails.

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

### Phase 1: core store and state

- state types
- SQLite schema and migrations
- event log
- state transition helpers
- artifact persistence helpers

### Phase 2: controller and scheduler

- mission controller
- scheduler
- lease logic
- recovery logic

### Phase 3: worker and review execution

- worktree manager
- worker execution mode
- review execution mode
- derived artifact generation

### Phase 4: integration and approvals

- integration engine
- post-apply validation
- approval lifecycle

### Phase 5: product surface

- CLI commands
- TUI mission board
- workflow panel integration
- status JSON

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

## 28. Explicit Anti-Patterns

The implementing agent must not do any of the following:

1. Implement mission orchestration as uncontrolled agent-to-agent freeform chat.
2. Introduce a daemon or remote protocol as a v1 requirement.
3. Merge worker changes without an independent review result.
4. Make artifact generation depend entirely on model obedience when deterministic generation is possible.
5. Add approval prompts that do not correspond to meaningful risk.
6. Create separate, conflicting notions of mission state across UI, runtime, and store.
7. Claim the feature is complete without deterministic tests and live smoke evidence.

---

## 29. Final Notes to the Implementing Agent

Build the smallest system that can credibly deliver:

- mission-level autonomy
- safe parallel execution
- trustworthy review
- durable recovery
- elegant operator control

If a proposed implementation makes the system feel more distributed than necessary, more magical than auditable, or more complicated than explainable, that implementation is wrong.

The correct v1 mission system should feel like a natural extension of Golem’s existing disciplined coding workflow, not like a separate platform stapled onto it.

# TUI Multi-Agent Orchestrator Draft Plan

## Goal

Turn Golem's existing TUI into a multi-agent orchestrator that feels as effective as Gastown/Gastown+Beads at parallel task execution, but is more logically structured, more auditable, and easier to reason about in code and in the UI.

## Grounding note

This draft is now informed by direct code inspection of:

- `steveyegge/gastown`
- `steveyegge/beads`

In particular, I inspected code around:

- Gastown Mayor orchestration (`internal/cmd/mayor.go`)
- Polecat spawning/reuse and worktree lifecycle (`internal/cmd/polecat_spawn.go`)
- Mailboxes and inter-agent messaging (`internal/mail/mailbox.go`)
- Convoy event polling and stranded-work recovery (`internal/daemon/convoy_manager.go`, `internal/cmd/convoy_launch.go`)
- Beads issue/task schema (`internal/types/types.go`)
- Beads tracker/plugin abstraction (`internal/tracker/tracker.go`)
- Beads agent state management (`cmd/bd/agent.go`)
- Gastown agent-bead fields (`internal/beads/beads_agent.go`)

## Working thesis

The right model is **not** a freeform swarm.
It is a **controller-led mission system** with:

- one durable orchestrator that owns truth
- many scoped worker runs that execute bounded tasks
- independent review runs that verify worker claims
- structured state, artifacts, and evidence in the TUI
- explicit operator gates only where they add safety

That gives us the upside of multi-agent throughput without the "agents talking past each other" failure mode.

## What we should preserve from Gastown / Beads

The code suggests a few concrete strengths worth preserving:

1. **Agent identity is durable even when sessions are not**
   - In Gastown, polecats are treated as persistent workers with durable identity and state, while the live session/process is ephemeral.
   - `internal/cmd/polecat_spawn.go` shows reuse of idle polecats and worktrees instead of always creating fresh workers.

2. **Work assignment is explicit, not implied**
   - Both repos encode ownership in durable state.
   - Gastown agent beads track `hook_bead` / `agent_state`; Beads agent commands expose explicit state transitions.

3. **Task tracking is a graph, not a checklist**
   - Beads' `Issue` type includes dependencies, parent/child structure, gate fields (`await_type`, `await_id`, `waiters`), slot ownership (`holder`), and agent-related fields.

4. **Agents communicate through structured artifacts**
   - Gastown mailboxes are backed by Beads issues/wisps, not just chat transcripts.
   - This is a strong signal that durable message/task artifacts beat freeform narration.

5. **The scheduler is both event-driven and recovery-oriented**
   - Gastown's convoy manager polls event streams and also runs stranded scans/recovery sweeps.
   - That is a good model for resilient orchestration.

6. **Parallel work is tied to concrete execution sandboxes**
   - Gastown polecats work in isolated worktrees and can be reused when safe.

## What we should improve

1. **Single source of truth**
   - No important state should exist only in model narration.
   - Mission/task/run state should be durable and inspectable.

2. **Deterministic coordination**
   - Agents should coordinate through task specs, ownership, artifacts, and verification.
   - Avoid open-ended cross-agent chatter as the primary protocol.

3. **Clear write boundaries**
   - Parallelism should depend on scoped file/component ownership.
   - Overlapping writable scope should be treated as a scheduling problem, not a model problem.

4. **Independent review before integration**
   - Worker output is evidence, not truth.
   - A separate reviewer should approve or reject work before integration.

5. **Operator comprehension**
   - The TUI should show mission phase, task graph, worker states, evidence, and blockers at a glance.

## Current assets we can build on

- Existing team/delegate support in the runtime, but it is tool-surface-driven and opportunistic today.
- Existing workflow proof surfaces already visible in the TUI:
  - plan
  - invariants
  - verification
  - team/workflow panel
- Existing mission-oriented design work already documented in `docs/mission-orchestration-prd.md`.
- Existing persistence-oriented thinking in `docs/persistent-loop-design.md`.

## What the inspected code suggests we should copy vs avoid

### Copy directly in spirit

1. **Beads-style structured task state**
   - Beads already models statuses, dependencies, gate waiting, slot ownership, metadata, assignee, external refs, and agent fields in one durable schema.
   - Golem should adopt a similarly explicit mission/task/run schema instead of inventing a looser chat-centric format.

2. **Gastown-style explicit worker state**
   - Agent state should be visible and durable: idle, spawning, running, working, stuck, done, stopped, dead.
   - A mission UI should not infer worker status from transcript timing.

3. **Gastown-style work hooks**
   - The useful idea is: every worker has one current assignment slot.
   - For Golem, that can be a task lease or `current_task_id` instead of literally copying `hook_bead` naming.

4. **Gastown convoy idea, but simplified**
   - Convoys are effectively tracked waves/batches of related work with dispatch and monitoring.
   - In Golem, that maps nicely to mission phases or task batches inside a DAG.

5. **Recovery loops, not just happy-path scheduling**
   - Convoy manager code shows warm-up, high-water marks, duplicate suppression, and stranded-work scans.
   - Golem should plan for restart/recovery from day one.

### Avoid copying wholesale in v1

1. **Do not make mailboxes the primary coordination model**
   - Gastown's mail/wisp system is powerful, but it adds another protocol layer.
   - For Golem v1, task artifacts and controller-owned state are simpler than agent-to-agent mail as the core mechanism.

2. **Do not start with tmux/session orchestration as the main abstraction**
   - Gastown has tmux, ACP, daemon, and session transition logic because it is operating a broader agent city.
   - Golem can stay controller-centric inside one TUI first, then add detached/background modes later.

3. **Do not import all of Beads' surface area immediately**
   - Beads contains issue tracking, external trackers, gates, slots, federation, and many agent-oriented affordances.
   - We should borrow the primitives, not the entire product.

## Proposed target architecture

### 1. Mission Controller

Add a first-class mission controller subsystem inside Golem.

Responsibilities:

- create mission from user goal
- produce and maintain the task graph
- assign ready tasks to workers
- enforce scope/dependency constraints
- launch review runs
- manage approvals
- decide when to replan
- maintain durable mission state
- drive the TUI presentation

This should be the canonical brain of orchestration, not the transcript.

### 2. Task Graph Instead of Agent Swarm

Represent work as a bounded DAG of tasks.

Each task should include:

- ID
- title
- intent / acceptance criteria
- dependencies
- writable scope
- read scope hints
- risk level
- required verification
- required invariants
- estimated execution mode
- status
- assigned run / worktree
- gate state
- lease holder / current worker
- structured metadata/artifacts

This is the key move toward "more logically sensible":
parallelism comes from graph structure and scope ownership, not from improvisational delegation alone.

### 2a. Borrow the right Beads primitives

Beads' code suggests that the minimal durable orchestration object should have more than title/status.

For Golem missions, each task should probably support these first-class primitives:

- **dependency edges** — from Beads dependencies
- **blocking state** — like Beads blocked / deferred workflows
- **gate fields** — analogous to `await_type`, `await_id`, and timeout/waiter concepts
- **exclusive slot / lease fields** — analogous to `holder`
- **assignment fields** — assignee / current worker / current task slot
- **metadata payloads** — for evidence, file scope, or integration info
- **agent state snapshots** — inspired by Beads/Gastown `agent_state`

That gives us an orchestration substrate with the same practical power, but with clearer mission semantics in the TUI.

### 3. Worker Runs

Workers are just existing Golem agent runs in a specialized execution mode.

Worker contract:

- receive one bounded task
- work in an isolated git worktree
- update plan/invariants/verification during execution
- produce structured artifacts on completion
- never integrate directly into the main mission branch

Expected output artifacts:

- summary
- changed files
- verification evidence
- unresolved risks
- patch/diff reference
- follow-up suggestions

### 4. Review Runs

Every completed worker task gets an independent review run before integration.

Reviewer contract:

- treat worker claims as untrusted
- inspect diff, task spec, and evidence
- run/inspect verification results
- produce pass/reject/request-changes result
- identify whether failure means retry, replan, or human gate

This is where we preserve effectiveness while improving trust.

### 5. Integration Engine

Do not make integration another freeform agent if we can avoid it.

Prefer deterministic integration logic that:

- waits for review pass
- checks dependency completion
- checks scope compatibility
- applies or merges approved diffs in a controlled order
- marks downstream tasks ready
- records the event trail

If agent help is needed, use it only for conflict resolution or re-planning.

### 6. Durable Mission Store

Back the controller with a local durable store.

Initial shape:

- mission table
- task table
- dependency table
- run table
- worker table
- artifact table
- approval/gate table
- event log table
- worktree lease table

SQLite is the obvious v1 choice.

The important lesson from Beads is not just "persist tasks" but "persist coordination primitives":

- dependencies
- gates
- holders/leases
- agent state
- structured metadata
- event history

If we skip those, we will recreate the usual swarm failure mode where the real state leaks back into prompt text.

### 7. Worktree Manager

Use separate git worktrees for concurrent writable tasks.

Rules:

- one active writable task per worktree
- task lease owns that worktree while running
- overlapping write scopes cannot run concurrently unless policy explicitly allows it
- cleanup is controller-managed and recoverable after restart

## TUI product design

### Replace the current workflow panel with a Mission Control layout when orchestration is active

Recommended panes:

1. **Mission header**
   - mission title
   - phase
   - objective
   - budget / elapsed time
   - active workers / queued tasks / blockers

2. **Task graph / task queue pane**
   - ready
   - running
   - in review
   - blocked
   - done

3. **Worker lane pane**
   - one compact row/card per worker
   - task ID
   - state
   - worktree
   - last event
   - verification badge

4. **Evidence pane**
   - latest review decision
   - failing verification
   - unresolved invariants
   - requested approvals

5. **Transcript pane**
   - stays useful, but becomes secondary to mission state

### New commands

- `/mission new <goal>`
- `/mission plan`
- `/mission start`
- `/mission pause`
- `/mission resume`
- `/mission cancel`
- `/mission status`
- `/mission approve <item>`
- `/mission reject <item>`
- `/task show <id>`
- `/task retry <id>`
- `/task replan <id>`
- `/worker show <id>`
- `/review show <task-id>`

## Execution model

### Phase 0: Single-run planner

Start with one planner run that turns the user goal into:

- mission summary
- success criteria
- task DAG
- task scopes
- initial invariants
- verification strategy
- approval gates

The user can inspect/approve this before execution if policy requires it.

### Phase 1: Controlled parallel workers

Scheduler repeatedly:

- selects ready tasks
- enforces scope/dependency limits
- launches workers into isolated worktrees
- streams worker state to the TUI
- captures structured artifacts

### Phase 2: Mandatory review

When a worker finishes:

- freeze worker artifacts
- launch review run
- allow integration only on pass
- otherwise route to retry, replan, or operator gate

### Phase 3: Replan loop

If tasks fail repeatedly, uncover new dependencies, or create follow-up work:

- planner revisits only affected branches of the graph
- controller records why replan happened
- TUI shows before/after graph delta

### Phase 4: Final integration + closeout

Mission completion should require:

- all required tasks completed
- all hard invariants passed
- required verification passed and not stale
- final mission summary artifact generated

## Role model

Keep the number of roles intentionally small.

### Required v1 roles

1. **Controller**
   - deterministic subsystem with occasional planner invocation
2. **Planner**
   - creates or revises the task graph
3. **Worker**
   - executes a bounded task
4. **Reviewer**
   - independently judges a worker result

### Avoid in v1

- manager agents talking to manager agents
- peer-to-peer worker coordination protocols
- always-on autonomous swarm chat
- too many anthropomorphic specialist roles

This keeps the mental model elegant.

## Scheduling policy

Priority order for runnable work:

1. dependency-ready tasks
2. highest value / lowest conflict tasks
3. tasks that unblock the most downstream work
4. tasks with exclusive writable scope availability
5. retries only if retry budget remains

Block execution when:

- write scope overlaps with active task
- required dependency incomplete
- missing approval gate
- mission budget exhausted
- repeated reviewer rejection exceeds threshold

## Artifact-first coordination

Agents should exchange structured artifacts, not conversational summaries.

Core artifact types:

- mission plan
- task spec
- worker result
- review result
- verification bundle
- replan decision
- integration event
- operator decision

This is the main design difference from a looser swarm and likely the biggest reliability win.

## Safety and trust model

1. No automatic push/PR/merge by default
2. Approval gates for destructive or ambiguous steps
3. Review required before integration
4. Verification recorded explicitly and can become stale
5. Mission recovery must work from durable state plus repo state
6. Controller must surface "why blocked" explicitly in UI

## Suggested implementation phases

### Phase A — Mission scaffolding in the TUI

Ship first:

- mission data model
- mission store
- mission status pane
- `/mission new`, `/mission status`
- planner-only flow that creates a task DAG

Success bar:
User can define a mission and inspect a durable plan in the TUI.

### Phase B — Worker orchestration

Ship next:

- task scheduler
- worktree manager
- worker launch lifecycle
- per-worker status cards
- task lease + event logging

Success bar:
Multiple bounded tasks can run concurrently without stepping on each other.

### Phase C — Review + controlled integration

Ship next:

- review runs
- review artifact model
- integration gate
- retry/replan flow

Success bar:
No worker result becomes mission truth without independent review.

### Phase D — Replanning and recovery

Ship next:

- partial graph replanning
- restart recovery
- paused/blocked/operator-action flows
- improved mission timeline UI

Success bar:
Long-running missions survive interruption and stay understandable.

### Phase E — Power features

Possible later:

- optional background daemon mode
- remote workers
- CI/PR hooks
- richer mission analytics

## Concrete first slice I would implement

If we want the most leverage with the least chaos, the first real implementation slice should be:

1. Add a durable `Mission` / `Task` / `Dependency` / `Worker` / `Run` model
2. Make task state include lease/holder, gate status, and evidence metadata from day one
3. Add `/mission new` that invokes a planner run and writes a task DAG
4. Add a mission-focused TUI pane that renders mission phase + tasks + blockers + worker states
5. Add a scheduler that can launch exactly one worker per ready task into a dedicated worktree
6. Add a mandatory reviewer step before task completion is accepted
7. Add a recovery sweep on mission resume/restart so stranded tasks are reclassified instead of silently lost

That is enough to prove the architecture without overcommitting to Gastown-level city complexity.

## Open design questions

1. Should mission state live entirely in Golem first, or should some primitives move into Gollem immediately?
2. Should review be mandatory for every task in v1, or only for tasks above a risk threshold?
3. What is the minimal useful task-scope language: explicit file globs, directories, or semantic components?
4. Do we want mission execution to remain foreground-only in v1, or support pause/resume across TUI restarts immediately?
5. How much of the current team/delegate surface should remain directly user-visible once mission orchestration exists?

## Recommendation

Use the existing `docs/mission-orchestration-prd.md` direction as the architectural backbone, but make the implementation explicitly informed by the Gastown/Beads codebase split:

- **from Beads**: durable graph state, dependencies, gates, slots/holders, agent/task metadata
- **from Gastown**: explicit worker identity/state, isolated worktrees, dispatch/recovery loops, visible orchestration roles
- **for Golem**: controller-centric Mission Control inside the TUI rather than a mail-driven or tmux-driven agent city

So the product should be:

- controller-centric
- graph-driven
- artifact-first
- review-gated
- worktree-isolated
- visibly auditable
- recovery-aware

That should get us the practical effectiveness of Gastown/Beads while being substantially easier to trust, debug, and extend inside Golem.

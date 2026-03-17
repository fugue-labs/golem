# Missions and Mission Control

Golem has two distinct ways to work:

- **Beginner / short sessions**: open `golem`, work directly in the main chat-first TUI, and use slash commands like `/help`, `/search <query>`, and `/doctor`.
- **Advanced / long-running work**: stay in the TUI for control, but switch into the durable **`/mission`** flow and monitor progress in the separate **Mission Control** dashboard.

This guide covers the shipped advanced workflow only. It is the right guide when you want tracked tasks, explicit approval gates, pause/resume controls, and a monitoring view that survives beyond a single transcript.

If you still need installation or authentication help, start with [Getting started with Golem](getting-started.md).

## When to use missions

Use a mission when the job is larger than a single prompt and you want durable state instead of a one-off chat exchange. Missions are a better fit when you want to:

- turn one goal into tracked tasks,
- inspect mission status and the task graph separately,
- review approval state before execution starts,
- pause and later resume work deliberately, or
- keep a second terminal open on progress while the main TUI remains your control surface.

If you are new to Golem, start with the normal `golem` TUI first. Move to missions when you need a more operator-driven workflow.

## Beginner path vs advanced path

### Beginner path

For normal interactive work, launch Golem and stay in chat:

```bash
golem
```

Useful first commands:

```text
/help
/search <query>
/doctor
```

This path is best when you want a quick coding session and do not need durable orchestration.

### Advanced path

For longer-running work, control the mission from the main TUI and watch it from another terminal:

```text
/mission new Build the feature and add verification
/mission plan
/mission approve
/mission start
```

Then open Mission Control separately:

```bash
golem dashboard
```

The main TUI is where you create, approve, start, pause, and cancel missions. The dashboard is where you monitor the durable mission state.

## The shipped `/mission` command surface

Inside the main TUI, the shipped mission commands are:

- `/mission new <goal>`
- `/mission status`
- `/mission tasks`
- `/mission plan`
- `/mission approve`
- `/mission start`
- `/mission pause`
- `/mission cancel`
- `/mission list`

That is the current operator-facing mission surface. Task-scoped retry, replan, or escalation controls discussed elsewhere in the repo are not the shipped command contract for this guide.

## Shipped mission flow

The shipped mission lifecycle is explicit on purpose: create, plan, approve, start, inspect, then pause or cancel when needed.

### 1. Create a draft mission

Create a durable mission from the main TUI:

```text
/mission new <goal>
```

This creates a mission in **`draft`** state. The mission exists durably, but it does not have a task graph yet.

**Operator expectation:** after `/mission new`, the normal next step is `/mission plan`.

### 2. Plan the mission

Run:

```text
/mission plan
```

Planning invokes the planner, applies a durable task graph, creates durable tasks and dependencies, records a mission-plan approval, and moves the mission to **`awaiting_approval`**.

**Operator expectation:** use planning to turn a goal into the tracked task DAG. After planning, inspect `/mission status` or `/mission tasks` before approving execution.

### 3. Approve the plan

Run:

```text
/mission approve
```

Approval resolves the durable mission-plan approval gate and immediately attempts to start execution. If some other approval is still required, Golem reports that the plan is approved but execution is still gated.

**Operator expectation:** approval is explicit. The shipped flow is designed so you can inspect the plan before the mission starts running.

### 4. Start or resume execution

Run:

```text
/mission start
```

`/mission start` is used for two shipped cases:

- start an `awaiting_approval` mission only after the required approvals are already resolved, or
- resume a mission that is currently **`paused`**.

`/mission start` does **not** bypass approval.

**Operator expectation:** if the mission is waiting on approvals, resolve those first. Resume is currently handled by `/mission start`; there is no separate `/mission resume` slash command.

### 5. Inspect durable mission state

The two primary in-TUI inspection commands are:

```text
/mission status
/mission tasks
```

`/mission status` is the high-level summary. It reports durable mission state such as:

- overall status,
- phase label,
- attention text,
- next action,
- focus task,
- queued next task,
- task-count summaries,
- active runs,
- approvals,
- blocked tasks, and
- ready or review queues.

`/mission tasks` is the task-graph view. It lists the task DAG with task IDs, statuses, titles or objectives, and dependency edges.

**Operator expectation:** use `/mission status` to answer “what is happening and what needs my attention?” Use `/mission tasks` to answer “what is the current DAG?”

### 6. Pause or cancel deliberately

From the main TUI, run either:

```text
/mission pause
/mission cancel
```

- `/mission pause` stops the in-process orchestrator so new work is not leased while the mission stays paused.
- `/mission cancel` stops the in-process orchestrator, marks the mission cancelled, and clears the active mission from the current TUI session.

**Operator expectation:** pause when you want to stop further execution without discarding mission state. Cancel when you want a terminal stop.

### 7. List known missions

Run:

```text
/mission list
```

This lists known missions and marks the mission that is active in the current chat session.

## Mission Control dashboard

Mission Control is the separate operator dashboard for durable missions.

Launch it from another terminal:

```bash
golem dashboard
```

You can also target a specific mission directly:

```bash
golem dashboard <mission-id>
```

### What the dashboard is for

Mission Control exists so you can inspect durable mission state even when the main transcript is not your current focus.

In practice:

- the main TUI is the control surface,
- `golem dashboard` is the monitoring surface, and
- keeping both open is the intended advanced workflow for long-running mission work.

If you do not supply a mission ID, the dashboard auto-selects the most relevant non-terminal mission in this priority order:

1. `running`
2. `blocked`
3. `paused`
4. `awaiting_approval`
5. `planning`
6. `draft`

## Dashboard layout and pane purpose

The shipped Mission Control layout has a header plus four panes:

- **Tasks**
- **Workers**
- **Evidence**
- **Events**

### Header

The header is the fast operator summary. It surfaces:

- mission status,
- task progress,
- active workers,
- pending approvals,
- evidence count,
- elapsed time,
- repo,
- branch, and
- worker budget.

Use it to answer the first question quickly: is the mission healthy, blocked, waiting for approval, or actively running?

### Tasks pane

The **Tasks** pane is the execution map. Use it to understand what work exists, what is ready, what is blocked, and how far the mission has progressed.

### Workers pane

The **Workers** pane shows worker activity. Use it to see what is currently in progress and whether active workers exist at all.

### Evidence pane

The **Evidence** pane is where approval and review-related signal is surfaced. Pending approvals appear here, along with review results, failures, and recorded artifacts.

### Events pane

The **Events** pane is the timeline view. Use it to follow durable mission events and operator-relevant progress over time.

## Dashboard navigation

Mission Control is keyboard navigable. The shipped hints are:

- `Tab` — switch panes
- `Shift+Tab` — switch panes in reverse
- `1-4` — jump directly to a pane
- `j/k` — scroll within the focused pane

These same hints are surfaced in `/help`, so the shell continues to teach the dashboard workflow.

## Empty-state and persistence expectations

Mission orchestration is local-first and persistence-backed. The dashboard reads durable mission state directly, so it can attach to existing mission data even when no active chat transcript is open.

When no mission is active, Mission Control still renders a valid empty state with guidance to create one using `/mission new`.

That means the advanced workflow is intentionally split:

- create and control missions from the main `golem` TUI, and
- monitor durable progress from `golem dashboard`.

## Recommended first advanced workflow

If you are trying missions for the first time, use this order:

1. Start the main TUI:

   ```bash
   golem
   ```

2. Create the mission:

   ```text
   /mission new Improve the feature and add tests
   ```

3. Plan it:

   ```text
   /mission plan
   ```

4. Inspect status or tasks:

   ```text
   /mission status
   /mission tasks
   ```

5. Approve it:

   ```text
   /mission approve
   ```

6. Start or resume it when appropriate:

   ```text
   /mission start
   ```

7. Open Mission Control in another terminal:

   ```bash
   golem dashboard
   ```

This keeps beginner work simple while giving advanced operators a clear, durable workflow for long-running repository changes.

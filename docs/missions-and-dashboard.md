# Missions and Mission Control

Golem has two different ways to work:

- **Beginner / short sessions**: open `golem`, ask for a change, and stay in the main chat-first TUI.
- **Advanced / long-running work**: use the durable **`/mission`** flow and the separate **Mission Control** dashboard.

This guide focuses on the shipped advanced workflow: creating a mission, planning it, approving it, running it, and watching progress in the dashboard.

## When to use missions

Use a mission when the work is bigger than a single prompt and you want durable state that survives beyond one transcript view. Missions are a better fit when you want to:

- break a larger goal into tracked tasks,
- inspect progress and blockers,
- pause and resume work deliberately,
- review approval state before execution starts, or
- monitor work from a separate terminal.

If you are just getting started, begin with the normal `golem` TUI and `/help`. Move to missions when you need a more operator-driven workflow.

## The shipped mission flow

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

You can always rediscover the flow from `/help`.

## Beginner path vs advanced path

### Beginner path

For a quick task, launch Golem and work directly in chat:

```bash
golem
```

Useful first commands:

```text
/help
/search <query>
/doctor
```

This is the fastest path when you do not need durable orchestration.

### Advanced path

For longer-running work, stay in the TUI but switch into the mission lifecycle:

```text
/mission new Build the feature and add verification
/mission plan
/mission approve
/mission start
```

Then open the dashboard in another terminal:

```bash
golem dashboard
```

## Mission lifecycle

The shipped lifecycle is intentionally explicit.

### 1. Create a draft mission

Create a durable mission from the main TUI:

```text
/mission new <goal>
```

This creates the mission in **`draft`** state. At this point the mission exists, but it does not have a task graph yet.

**Operator expectation:** after creating a mission, the normal next step is `/mission plan`.

### 2. Plan the mission

Run:

```text
/mission plan
```

Planning moves the mission through the planner and applies a durable task graph. Once the plan is applied, the mission moves to **`awaiting_approval`**.

**Operator expectation:** planning is the normal path from a new draft mission to tracked tasks. If you want to inspect the resulting work, use `/mission status` and `/mission tasks`.

### 3. Review and approve

Run:

```text
/mission approve
```

Approval resolves the durable mission-plan approval gate and immediately attempts to start execution. If some other approval still blocks execution, Golem reports that the plan is approved but the mission is still gated.

**Operator expectation:** approval is not implicit. The mission flow is designed so you can inspect the plan before work begins.

### 4. Start or resume execution

Run:

```text
/mission start
```

`/mission start` is used for two shipped cases:

- start an already approved mission that is waiting to run, or
- resume a mission that is currently **`paused`**.

`/mission start` does **not** bypass approval.

**Operator expectation:** if approval is still pending, approve first.

### 5. Inspect progress

Two commands are the main status surfaces in the TUI:

```text
/mission status
/mission tasks
```

`/mission status` summarizes durable mission state, including status, phase, next action, attention text, focus information, and related counts.

`/mission tasks` shows the current task DAG details, including task IDs, statuses, titles/objectives, and dependency edges.

**Operator expectation:** use `/mission status` for the high-level answer to “what is happening?” and `/mission tasks` for the detailed answer to “what is the task graph?”

### 6. Pause or cancel when needed

Run either of these from the TUI:

```text
/mission pause
/mission cancel
```

- `/mission pause` stops new task leasing so the mission remains paused.
- `/mission cancel` cancels the mission and clears the active mission from the current TUI session.

**Operator expectation:** pausing is the safe control for stopping further execution without throwing away mission state. Cancelling is the terminal stop.

### 7. List known missions

Run:

```text
/mission list
```

This shows known missions and marks the mission that is active in the current chat session.

## Mission Control dashboard

The dashboard is a separate operator surface for durable missions.

Launch it from another terminal:

```bash
golem dashboard
```

You can also target a specific mission directly:

```bash
golem dashboard <mission-id>
```

### What Mission Control is for

Mission Control is the dashboard for long-running mission work. It exists so you can inspect durable mission state even when you are not focused on the main transcript.

In practice, that means:

- the main TUI is where you create and control missions,
- the dashboard is where you monitor them, and
- you can keep the dashboard open in another terminal while work continues.

If no mission ID is supplied, the dashboard auto-selects the most relevant non-terminal mission. The shipped priority is:

1. `running`
2. `blocked`
3. `paused`
4. `awaiting_approval`
5. `planning`
6. `draft`

## Dashboard layout

The shipped dashboard layout is:

- a **Mission Control** header,
- a mission summary section, and
- four panes:
  - **Tasks**
  - **Workers**
  - **Evidence**
  - **Events**

### Header

The header surfaces operator-facing mission context such as:

- mission status,
- task progress,
- active workers,
- pending approvals,
- evidence count,
- elapsed time,
- repo,
- branch, and
- worker budget.

This is the fastest place to answer “is the mission healthy, blocked, or waiting on me?”

### Tasks pane

The **Tasks** pane is the execution map. Use it to understand what work exists, what is ready, what is blocked, and how far the mission has progressed.

### Workers pane

The **Workers** pane shows current worker activity. Use it to see what is actively being worked on and whether there are active workers at all.

### Evidence pane

The **Evidence** pane is where approval and review-related signal shows up. Pending approvals are surfaced here, along with other review results, failures, and recorded artifacts.

### Events pane

The **Events** pane is the mission timeline. Use it to see durable state changes and operator-relevant activity as the mission progresses.

## Dashboard navigation

Mission Control is designed to be navigated directly from the keyboard. The shipped navigation hints are:

- `Tab` — switch panes
- `Shift+Tab` — reverse pane switch
- `1-4` — jump directly to a pane
- `j/k` — scroll within the focused pane

These hints also appear in Golem’s help copy so the dashboard remains discoverable from the main shell.

## Empty-state behavior

The dashboard is still useful before work starts. If there is no active mission, Mission Control shows an explicit empty state instead of failing silently.

The important operator expectation is simple: create a mission from the main TUI with `/mission new`, then reopen or refresh the dashboard.

## Operator expectations for long-running work

The mission system is designed for deliberate control, not fire-and-forget magic. The practical expectations are:

- **You create the mission from the main TUI.**
- **You plan before execution starts.**
- **You approve before execution starts.**
- **You use `/mission start` to begin or resume work.**
- **You use `/mission status` and `/mission tasks` for in-shell inspection.**
- **You use `golem dashboard` for a separate monitoring view.**
- **You pause when you want to stop new work cleanly.**
- **You cancel when you want to end the mission.**

In short: beginners can stay in the main chat flow, while advanced operators can move to durable missions plus Mission Control when they need a longer-running, inspectable workflow.

# TUI Best Practices Audit

## Purpose

This audit compares the current shipped TUI surfaces against the local Charmbracelet skill guidance and the repo's existing UX contract before any implementation changes begin.

### Sources reviewed

- `internal/ui/app.go`
- `internal/ui/chat/messages.go`
- `internal/ui/workflow_panel.go`
- `internal/ui/dashboard/dashboard.go`
- `internal/ui/styles/styles.go`
- `docs/features.md`
- `test/e2e/tuistory_test.go`
- `/Users/trevor/.claude/skills/charmbracelet-tui/SKILL.md`
- `/Users/trevor/.claude/skills/charmbracelet-tui/references/polish-checklist.md`
- `/Users/trevor/.claude/skills/charmbracelet-tui/references/layout-patterns.md`
- `/Users/trevor/.claude/skills/charmbracelet-tui/references/crush-patterns.md`
- `/Users/trevor/.claude/skills/charmbracelet-tui/references/templates/dashboard.md`

## Non-regression rule

This audit is advisory. The current UX contract in `docs/features.md` and the observable behavior already covered by `test/e2e/tuistory_test.go` must remain stable through any polish pass.

### Preserve these operator-visible behaviors

- The main shell must keep a visible `GOLEM` anchor plus a visible prompt/input affordance.
- `/help` must keep surfacing the tested slash commands and key hints, including `/search <query>` guidance.
- `/search` with no args must keep its usage copy, including `search across all saved sessions` and `Examples`.
- `/mission` help/new/status/list flows must remain intact and readable.
- `/rewind` and `/replay` must keep their empty-state and error/help copy stable enough for current e2e assertions.
- Unknown slash commands must still fail obviously.
- `Esc` cancellation, `PgUp/PgDn` transcript scrolling, `↑/↓` input history, `/clear`, `/model`, `/doctor`, `/cost`, and slash-command tab completion must remain stable.
- `golem dashboard` must still land in `Mission Control` or a valid no-mission/error state and retain pane navigation (`Tab`, `Shift+Tab`, `1-4`, `j/k`, `q`).
- File-watch activity must not destabilize the shell or remove the visible `GOLEM` anchor.

## Executive summary

| Surface | Current strengths | Highest-confidence gaps | Recommended first moves |
|---|---|---|---|
| App shell | Clear `GOLEM` identity, strong launch guidance, resize handling, compact fallback, persistent status/help framing | No explicit minimum-size state, fixed palette instead of `LightDark`, several hardcoded layout thresholds, manual transcript scrolling instead of a viewport | Add minimum-size handling, centralize sizing constants, introduce adaptive colors, preserve launch/help/status contract |
| Workflow rail | Good prioritization of mission/spec/plan/verify/invariants, strong icon language, hidden on narrow terminals to protect chat width | Dense content at 38 columns, limited next-action guidance, passive-only rail, no narrow-mode summary besides status/header text | Keep current rail gating, but improve blocked/approval emphasis, add operator next-step cues, define shared sizing rules |
| Chat rendering | Strong role separation, markdown assistant rendering, inline tool results, explicit live/error/system states | Long sessions can still feel dense, truncation is aggressive, system summaries flatten multiple system message types, manual scroll path limits future polish | Improve vertical rhythm and summary taxonomy, consider viewport-based transcript, keep existing `/help`/search/help/cancel behaviors untouched |
| Dashboard | Stable Mission Control identity, four-pane model, usable keyboard navigation, clear empty states, rich mission metrics | No explicit small-terminal mode, plain-text error state, fixed split ratios, focus treatment is serviceable but not very strong | Add responsive layout + minimum-size state, strengthen focus styling, make loading/error/empty cards visually consistent |

## Cross-surface checklist mapping

| Charmbracelet checklist item | Current state | Evidence | Audit call |
|---|---|---|---|
| `View() -> tea.View` | Strong | `app.go` and `dashboard.go` return `tea.View` via `tea.NewView(...)` | Keep |
| `tea.KeyPressMsg` | Strong | Main app and dashboard both use `tea.KeyPressMsg` | Keep |
| Loading / empty / error states | Partial-strong | Main shell has welcome/loading states; dashboard has empty and error states | Improve visual consistency, not semantics |
| `WindowSizeMsg` responsiveness | Strong | Main shell and dashboard both recalculate width/height on resize | Keep |
| Minimum terminal size handling | Gap | No explicit `Terminal too small` state in audited surfaces | Add |
| Adaptive `LightDark` colors | Gap | Styles use a fixed Charmtone palette and ignore background color input beyond initialization | Add |
| Clear help visibility | Strong | Welcome, status bar, `/help`, input placeholder, dashboard footer all teach keys | Keep |
| Not color-only status | Strong | Icons and labels are paired (`✓`, `◐`, `✗`, `LIVE`, `ERROR`, etc.) | Keep |
| Viewport for long content | Gap/partial | Main transcript and dashboard scroll manually, not through a viewport component | Consider for polish pass |
| Consistent layout constants | Partial | Several layout widths/heights are coherent but hardcoded (`38`, `110`, split ratios) | Centralize |

## Surface audit

### 1. Main app shell (`internal/ui/app.go`, `internal/ui/styles/styles.go`)

#### Current strengths

- The shell already has a strong first frame: two-line header, transcript, always-visible input, and persistent `GOLEM` status region.
- Launch guidance is good. The welcome view teaches `/help`, `/search <query>`, `/doctor`, shell regions, and core keys before the operator does anything.
- Resize behavior is already in place via `tea.WindowSizeMsg`, with a compact fallback when full chrome does not fit.
- The shell keeps discoverability distributed across the product, not only in `/help`: placeholder text, header activity, welcome copy, and status hints all teach next actions.
- Status and workflow summaries are already integrated into the chrome instead of being buried in transcript text.
- The app requests background color and initializes styles early, which is a good Bubble Tea v2 pattern.

#### Concrete gaps vs. Charmbracelet guidance

- **No explicit minimum-size state.** The app falls back to compact rendering, but there is no clear `terminal too small` message or hard floor for unusable sizes.
- **Palette is not adaptive yet.** `styles.New` currently uses a fixed Charmtone-derived palette; it does not apply `lipgloss.LightDark(...)` based on detected background, despite requesting background color.
- **Layout constants are still embedded in rendering logic.** Examples include the workflow rail width gate and several width/height thresholds in shell rendering.
- **Transcript rendering is hand-managed.** That works today, but it leaves less room for future polish such as smooth long-form scrolling, scroll percentages, and shared viewport behaviors.
- **Advanced terminal affordances are still limited.** The shell uses `AltScreen`, but not mouse mode, focus reporting, or window title behavior from the skill examples.

#### Recommended changes

1. Add an explicit minimum-size fallback before any deeper polish so the shell fails gracefully on tiny terminals.
2. Refactor `styles.New` toward adaptive `LightDark` colors while preserving existing semantic color meanings and contrast.
3. Move shell sizing thresholds into named constants shared by header/transcript/input/status composition.
4. If transcript polish expands, prefer a viewport-backed path rather than adding more manual scroll logic.
5. Treat mouse/focus/window-title support as optional polish, not as a prerequisite for the current UX contract.

#### Must preserve while changing

- Visible `GOLEM` anchor.
- Prompt affordance (`❯` or placeholder copy).
- `/help` discoverability.
- Status-bar hints for send/cancel/scroll/complete.
- Existing scroll, history, cancellation, and command sequencing behavior already covered by e2e tests.

### 2. Workflow rail (`internal/ui/workflow_panel.go`, `internal/ui/mission_panel.go`)

#### Current strengths

- The rail already follows a useful operational hierarchy: mission/spec/plan/verification/invariants/team are prioritized by urgency, not just by data source.
- The current icon system is good and not color-only: `✓`, `◐`, `✗`, `○`, `●` make state legible even without perfect color support.
- The rail protects transcript readability by only appearing on wider terminals.
- Section headers already emphasize active work and spotlight a focus item rather than dumping raw task lists.
- Mission-specific summaries surface approvals, blockers, next tasks, active workers, and counts in a compact operator-oriented way.

#### Concrete gaps vs. Charmbracelet guidance

- **38-column density pressure.** The rail frequently compresses nuanced states into very little width, so blockers, approval context, and notes can truncate quickly.
- **Next action is unevenly expressed.** Some sections spotlight the current item well, but the operator's next command or decision is not always explicit.
- **Sizing logic is heuristic-heavy and local.** Target line budgets and priorities are effective, but they are not yet codified as a reusable layout policy.
- **Hidden-on-narrow behavior has no explicit substitute rail summary.** The main shell still exposes status, but the workflow story becomes more implicit once the panel disappears.
- **The rail is read-only/pseudo-passive.** That is acceptable today, but it means focus and navigation affordances are much weaker than in surfaces that need operator control.

#### Recommended changes

1. Keep the current width gate, but add a stronger narrow-mode summary in header/status when the rail is suppressed.
2. Standardize section headers around `state + why it matters + next operator move`.
3. Reserve more of the visible line budget for blockers, approvals, and in-progress work before secondary counts.
4. Promote blocked/approval sections with stronger visual framing, not only different text.
5. Consolidate rail widths, priorities, and target heights into named constants so layout changes stay intentional.

#### Must preserve while changing

- The rail must remain secondary to the transcript, not overwhelm it.
- Mission/spec/plan/verify/invariant states must keep reading as active workflow, not as raw JSON-like detail.
- Existing tested shell behavior must remain stable even if workflow copy is clarified.

### 3. Chat rendering (`internal/ui/chat/messages.go`)

#### Current strengths

- Roles are already well distinguished: `USER`, `ASSISTANT`, `TOOL`, `THINKING`, `SUMMARY`, and `ERROR` all have separate rendering paths.
- Assistant messages use markdown rendering, which is aligned with Charmbracelet guidance for rich terminal output.
- Tool calls and results are treated as first-class transcript citizens, with inline states, durations, and specialized renderers for view/edit/write/bash/grep/glob/ls output.
- Long tool output is truncated intentionally instead of flooding the transcript.
- Streaming state is surfaced clearly with `LIVE`, and tool activity includes running/success/error labels.
- Error rendering is explicit and readable instead of relying on raw log dumps.

#### Concrete gaps vs. Charmbracelet guidance

- **Long-session density still accumulates.** Even with tags and spacing, large transcripts can become visually heavy because everything remains in one manual scroll stream.
- **Truncation is one-way.** It protects the layout, but there is no operator affordance in the chat surface to expand or inspect more detail from the transcript itself.
- **System summaries are visually unified.** `SUMMARY` is useful, but multiple system-message categories can collapse into one visual treatment.
- **Spacing/gutter hierarchy could be stronger.** User, assistant, tool, and system blocks are differentiated, but not yet at the level of best-in-class transcript rhythm.
- **The transcript does not yet use a viewport abstraction.** That limits future polish around scroll position, jump-to-bottom behavior, and efficient long-session rendering.

#### Recommended changes

1. Increase vertical rhythm between major message classes without changing the tested textual content.
2. Introduce a clearer taxonomy for system summaries, especially for usage, verification, and lifecycle summaries.
3. Revisit tool-output truncation with an eye toward preserving scannability while making deeper inspection possible.
4. If transcript complexity grows further, move toward a viewport-backed implementation rather than expanding manual line slicing.
5. Preserve the current observable text that e2e tests rely on, especially around `/help`, `/search`, errors, and resilient message sending.

#### Must preserve while changing

- User text must remain visible immediately after send.
- Assistant/error/tool/system output must remain readable in plain terminal snapshots.
- Current slash-command outputs and error phrases used by e2e tests must remain recognizable.
- `Esc` cancel, page navigation, and input history must stay readable while transcript styling evolves.

### 4. Dashboard (`internal/ui/dashboard/dashboard.go`)

#### Current strengths

- `Mission Control` is a strong, stable identity anchor and already matches the repo UX contract.
- The dashboard keeps a coherent four-pane model: Tasks, Workers, Evidence, and Events.
- Keyboard navigation is already good: `Tab`, `Shift+Tab`, `1-4`, `j/k`, `r`, and `q` are visible and implemented.
- Empty states are explicit and helpful across header, worker, evidence, and event panes.
- Header metrics give a strong operator summary: task progress, workers, approvals, evidence, elapsed time, repo, branch, and budget.
- Pane headers include visible focus treatment (`▸`, `ACTIVE`, pane shortcuts), which is a solid base.

#### Concrete gaps vs. Charmbracelet guidance

- **No explicit small-terminal mode.** The layout assumes a multi-pane grid and clips itself into that structure even when the terminal is constrained.
- **Error state is visually plain.** `Dashboard error: ... Press q to quit.` is serviceable but not consistent with the rest of the panelized UI language.
- **Split ratios are fixed.** The 3/5 vs 2/5 split and internal height splits work, but they do not yet adapt into a stacked narrow layout.
- **Loading treatment is minimal.** The template guidance favors a clearer loading rhythm; current behavior is closer to `Loading...` and refresh polling.
- **Focus treatment could go further.** The current pane indicator is functional, but the active pane is not yet visually distinct enough to feel premium.
- **Palette adaptation gap also applies here.** Like the main shell, the dashboard uses a fixed palette rather than true light/dark adaptation.

#### Recommended changes

1. Add explicit minimum-size handling and a narrow stacked layout before adding new dashboard content.
2. Keep the four-pane information model, but make pane arrangement responsive.
3. Convert loading and error states into the same visual card language as the rest of Mission Control.
4. Strengthen active-pane styling beyond the current arrow/meta treatment.
5. Consider dashboard-template ideas like clearer metric grouping and richer summary cards only after responsiveness and state consistency are solved.

#### Must preserve while changing

- `Mission Control` identity.
- Valid no-mission state with creation guidance.
- Four-pane information model.
- Tab/Shift+Tab/1-4/j/k/q navigation.
- Header metrics and operator-facing durable mission summary behavior already described in `docs/features.md`.

## Recommended implementation order

### P0: Safe polish foundations

1. Add minimum-size handling to both main shell and dashboard.
2. Convert styles toward adaptive `LightDark` behavior without changing status semantics.
3. Centralize shell/rail/dashboard sizing constants.

### P1: Readability improvements

1. Strengthen workflow rail emphasis for blockers, approvals, and next action.
2. Improve transcript spacing and system-summary taxonomy.
3. Improve dashboard focus treatment and error/loading card consistency.

### P2: Structural upgrades

1. Introduce responsive stacked dashboard behavior for narrow terminals.
2. Evaluate viewport-backed transcript/dashboard scrolling.
3. Revisit tool-output expansion/collapse patterns once the base layout is stable.

## Implementation guardrail

Any future TUI implementation task should explicitly cite:

1. the surface being changed,
2. the relevant Charmbracelet checklist items,
3. the exact UX contract bullets in `docs/features.md` that cannot regress, and
4. the matching `tuistory` e2e assertions that must still pass.

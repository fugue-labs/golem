# Implementation Spec Closure Plan

This document turns the current gap analysis into an execution backlog for bringing the codebase into tighter alignment with `docs/implementation-spec.md`.

## 1. Summary

The codebase already satisfies much of the spec's core execution model:

- runtime preparation and routing
- plan / invariant / verification state mirroring
- queued follow-up prompts while busy
- stale-run filtering
- clear / cancel / session cleanup
- workflow rendering in the TUI
- upstream Gollem-powered higher-level tools through `codetool.AgentOptions`

The highest-value remaining work is not a rewrite. It is a focused closure effort around:

1. startup validation and reporting
2. prompt/runtime/tool-surface parity
3. lifecycle and error-path hardening
4. eval and golden-test infrastructure
5. final acceptance review against the spec

The persistent loop / daemon design in `docs/persistent-loop-design.md` should be treated as a separate roadmap track unless explicitly reprioritized.

## 2. Current State vs Spec

## 2.1 Already aligned enough to keep

These areas are already substantially implemented and should be preserved while tightening edges:

- Runtime preparation and effective team-mode resolution
- Router fallback without crashing the run
- Persistent session reuse across interactive turns
- Tool-state mirroring into UI plan / invariants / verification state
- Busy-state prompt queueing
- Clear/reset/cancel lifecycle handling
- Workflow visibility in the UI
- Verification freshness tracking
- Higher-level tool exposure through Gollem codetool integration

## 2.2 Main gaps to close

1. **Startup/config validation is weaker than the spec requires**
   - The app loads config, but does not perform a strong explicit validation pass before starting the TUI.
   - There is no first-class machine-readable runtime/status output.

2. **Prompt/runtime/tool disclosure has duplication risk**
   - Runtime/tool-surface reporting is repeated across model-facing and user-facing surfaces.
   - The spec explicitly calls for avoiding contradictory or redundant prompt guidance.

3. **Lifecycle testing is solid but not complete**
   - Happy-path tests exist, but some negative/error-path and edge-case coverage should be expanded.

4. **Eval and golden-test infrastructure is missing**
   - The spec calls for a first-class eval suite, run summaries, and regression capture.
   - The repo does not yet contain a dedicated eval harness or golden-fixture structure.

5. **Acceptance criteria need a final pass**
   - After targeted work, the implementation should be re-audited against the spec document and only then considered complete.

## 3. Execution Order

Recommended order:

1. PR1 — Runtime validation and reporting
2. PR2 — Prompt/runtime/tool-surface parity
3. PR3 — Lifecycle/UI hardening
4. PR4 — Eval/golden infrastructure
5. PR5 — Acceptance audit and roadmap boundary review

This order is intentional:

- PR1 and PR2 reduce product-level ambiguity first.
- PR3 locks in behavioral correctness before wider evaluation work.
- PR4 adds durable regression machinery after the runtime contract is clearer.
- PR5 is the final gate.

## 4. Detailed Backlog

## PR1 — Runtime validation and machine-readable reporting

### Goal

Meet spec requirements around startup correctness, explicit runtime validation, and visible effective runtime reporting.

### Spec sections addressed

- §6.1 Startup and configuration
- §6.2 Runtime preparation
- §16 Observability and diagnostics
- §17 Security and safety constraints

### Deliverables

1. Add a first-class config/runtime validation pass.
2. Distinguish fatal validation errors from informational runtime notes.
3. Introduce a shared structured runtime profile/report object as the source of truth for runtime disclosure.
4. Add machine-readable runtime/status output.
5. Ensure startup failure messages are actionable.

### Concrete tasks

#### 1. Add config validation API

Files:

- `internal/config/config.go`
- `internal/config/config_test.go`

Tasks:

- Add `Validate()` or equivalent on `Config`.
- Preserve raw user input for normalized settings (at minimum `GOLEM_TEAM_MODE`) or validate those inputs before normalization so invalid values remain detectable.
- Validate provider-specific requirements, such as:
  - missing API key / auth material
  - missing Vertex project/region when required
  - invalid team mode values
  - invalid timeout / auto-context combinations if relevant
- Separate:
  - fatal startup blockers
  - non-fatal warnings / notes

Suggested shape:

- `type ValidationResult struct { Errors []string; Warnings []string }`
- `func (c *Config) Validate() ValidationResult`
- optional raw-input field or helper for pre-normalization env validation where needed

#### 2. Call validation at startup

Files:

- `main.go`

Tasks:

- Run config validation after `config.Load()` and before TUI startup.
- If validation has fatal errors, print them cleanly and exit non-zero.
- If validation only has warnings, either:
  - print them once before entering the TUI, or
  - inject them into the initial runtime/profile surface.

#### 3. Add machine-readable status/runtime output

Files:

- `main.go`
- `internal/config/config.go`
- `internal/agent/runtime_state.go`
- possibly a small new helper file such as `internal/agent/runtime_report.go`

Tasks:

- Extend CLI support with a machine-readable mode, for example:
  - `golem status --json`
  - `golem runtime --json`
- Return structured data for:
  - provider/model
  - router model
  - effective team mode
  - team-mode reason
  - code mode status
  - open-image status
  - timeout
  - auto-context settings
  - validation warnings
- Drive this output from the shared runtime profile/report object introduced in PR1 rather than from separate ad hoc structs.

#### 4. Introduce the shared runtime reporting source of truth

Files:

- `main.go`
- `internal/config/config.go`
- `internal/agent/runtime_state.go`
- new helper file such as `internal/agent/runtime_profile.go` or `internal/agent/runtime_report.go`

Tasks:

- Define one structured representation for runtime + tool capability disclosure.
- Feed it from the real config/runtime decisions, including validation warnings.
- Use it to drive startup reporting and CLI status/runtime surfaces in PR1.
- Treat this as the only runtime-reporting data model; PR2 should consume it for prompt/UI parity rather than redefining it.

### Tests

Add or extend tests for:

- missing credentials per provider
- invalid team-mode values
- validation warnings vs hard failures
- `status --json` shape and content
- runtime-preparation report content

### Acceptance criteria

- Startup fails fast on invalid required credentials/settings.
- Status/runtime can be surfaced in both human-readable and machine-readable forms.
- Effective runtime values match actual runtime decisions.

## PR2 — Prompt/runtime/tool-surface parity

### Goal

Make tool/runtime disclosure consistent across:

- static system prompt
- dynamic runtime prompt
- UI runtime summary
- actual tool availability

### Spec sections addressed

- §11 System Prompt and Behavioral Contract
- §11.1 Prompt composition contract
- §11.2 Prompt maintenance requirements
- §12 Tooling specification

### Deliverables

1. Adopt PR1's shared runtime/tool profile across prompt and UI surfaces.
2. Reduced duplication between static and dynamic prompt layers.
3. Clear behavior when a tool/capability is unavailable.
4. Regression tests to prevent contradiction drift.

### Concrete tasks

#### 1. Adopt the PR1 runtime/tool capability description

Files:

- `internal/agent/agent.go`
- `internal/ui/commands.go`
- the shared helper introduced in PR1

Tasks:

- Consume the structured runtime profile/report object added in PR1.
- Use that same object to drive:
  - model-facing runtime prompt section
  - `/runtime` output in the UI
- Do not create a second runtime-report representation in PR2.

#### 2. Reduce prompt duplication

Files:

- `internal/agent/system_prompt.md`
- `internal/agent/agent.go`

Tasks:

- Keep stable policy in the static prompt.
- Keep dynamic facts only in the runtime prompt.
- Remove or simplify repeated tool/runtime bullets where they are duplicated unnecessarily.
- Ensure tool descriptions do not imply capabilities that are only conditionally available.

#### 3. Audit tool availability disclosure

Files:

- `internal/agent/agent.go`
- possibly helper files under `internal/agent/`

Tasks:

- Explicitly account for:
  - delegate on/off
  - execute_code on/off/unavailable
  - open_image on/off/pending
  - router model fallback notes
- Make disclosure match Gollem codetool behavior rather than local `internal/agent/tools` alone.

#### 4. Add prompt regression tests

Files:

- `internal/agent/agent_test.go`
- maybe add a new golden file directory if desired in PR4

Tasks:

- Assert no contradictory tool-surface claims.
- Assert no duplicated labels in runtime summary.
- Assert expected presence/absence of optional capabilities based on runtime state.

### Tests

Add tests for:

- delegate disabled
- code mode unavailable
- vision-supported vs text-only model
- router fallback note rendering
- prompt composition with/without top-level personality and skills

### Acceptance criteria

- UI and model see the same runtime/tool truth.
- Prompt composition is factual and non-contradictory.
- Tool-surface regressions are caught by tests.

## PR3 — Lifecycle/UI hardening and edge-case coverage

### Goal

Strengthen confidence in run lifecycle handling, especially around errors, cancellation, stale events, queued input, and final tool-state propagation.

### Spec sections addressed

- §7 State management requirements
- §8 Concurrency and async behavior
- §10 End-to-end execution flow
- §13 UI specification
- §15 Testing strategy

### Deliverables

1. Broader integration-style test coverage for the TUI model.
2. Better coverage of failure and race-adjacent cases.
3. Confidence that workflow state remains correct under interruption.

### Concrete tasks

#### 1. Expand stale-run filtering tests

Files:

- `internal/ui/app_test.go`
- `internal/ui/state_flow_test.go`

Tasks:

- Add explicit tests that stale:
  - text deltas
  - thinking deltas
  - tool calls
  - tool results
  - runtime prepared messages
  - completion messages
  are all ignored when `runID` no longer matches.

#### 2. Expand cancel/clear coverage

Files:

- `internal/ui/workflow_panel_test.go`
- `internal/ui/app_test.go`

Tasks:

- Add tests for:
  - cancellation while prompts are queued
  - clear during/after busy state
  - session cleanup semantics
  - preservation vs reset of workflow state in the correct scenarios

#### 3. Expand error-path runtime-preparation tests

Files:

- `internal/ui/app_test.go`
- `internal/agent/runtime_state.go`
- `internal/agent/router_test.go`

Tasks:

- Test runtime-preparation failure messaging.
- Test router-model fallback behavior surfaced to the UI.
- Test agent construction failure after runtime preparation.

#### 4. Expand workflow mirroring tests

Files:

- `internal/ui/state_flow_test.go`
- `internal/ui/workflow_panel_test.go`
- `internal/ui/verification/verification_test.go`

Tasks:

- Cover mixed plan + invariant + verification updates across:
  - tool result messages
  - final run completion
  - mutating tool-induced staleness
- Verify that final workflow state remains visible even after failures.

### Tests

- Add focused model-update tests instead of relying only on broad rendering checks.
- Prefer deterministic tests around `Update(...)` messages and lifecycle transitions.

### Acceptance criteria

- Stale events are consistently ignored.
- Queue/cancel/clear behavior is stable and tested.
- Workflow state remains accurate across success, failure, and cancellation.

## PR4 — Eval, golden tests, and run-summary capture

### Goal

Add the regression infrastructure the spec expects, so product quality is measurable rather than anecdotal.

### Spec sections addressed

- §14 Evaluation framework
- §15.3 Golden tests
- §16 Observability and diagnostics
- §18 Rollout plan phase 3

### Deliverables

1. Eval case schema.
2. Run-summary / transcript / tool-trace capture.
3. Golden tests for stable text surfaces.
4. Small curated benchmark set.

### Concrete tasks

#### 1. Define eval schema

Suggested location:

- `internal/eval/` or `eval/`

Suggested entities:

- `EvalCase`
- `EvalRunSummary`
- `ToolTraceEntry`
- `VerificationSummary`

Fields should cover:

- prompt
- runtime/provider/model settings
- transcript
- tool call sequence
- changed files
- verification commands run
- final status
- rubric notes

#### 2. Capture run summaries

Files:

- likely `internal/ui/app.go`
- possibly new package under `internal/eval/` or `internal/runsummary/`

Tasks:

- Add a structured summary object emitted at run completion.
- Include enough information to support both manual debugging and future eval harness execution.
- Keep persistence optional if desired, but make the structure exist in code.

#### 3. Add golden tests

Suggested targets:

- runtime prompt output
- `/help` rendering
- `/runtime` output
- workflow panel summaries

Possible locations:

- `internal/agent/testdata/`
- `internal/ui/testdata/`

Tasks:

- Generate golden fixtures.
- Add helper(s) for stable normalization where necessary.
- Ensure tests fail with readable diffs.

#### 4. Add a curated benchmark set

Suggested categories:

- prompt-only brief chat case
- repo-introspection/doc-writing case
- verification-required coding case
- failure-recovery case

This does not need to be a huge harness in v1; it needs to establish the pattern.

### Tests

- Golden tests should be deterministic and insensitive to timestamps.
- Eval schemas should have unit tests for serialization and summary generation.

### Acceptance criteria

- The repo contains a real eval/golden structure.
- Stable user/model-facing surfaces are regression-tested.
- Run results can be captured in structured form.

## PR5 — Final acceptance audit and roadmap split

### Goal

Determine whether the repo now satisfies the implementation spec and keep the persistent-loop design from muddying that milestone.

### Spec sections addressed

- §18 Rollout plan
- §19 Concrete implementation backlog
- §20 Acceptance criteria
- §22 Recommended next steps

### Deliverables

1. A final acceptance checklist against `docs/implementation-spec.md`.
2. A documented decision that `docs/persistent-loop-design.md` is:
   - either out of scope for current spec closure,
   - or promoted into active implementation scope.

### Concrete tasks

#### 1. Build a checklist from spec acceptance criteria

Use `docs/implementation-spec.md` §20 directly and mark each item:

- pass
- partial
- fail
- deferred

#### 2. Reconcile open questions vs blockers

Tasks:

- Distinguish real blockers from optional design questions.
- Move non-blocking future work into roadmap docs instead of leaving ambiguity.

#### 3. Decide loop/daemon scope explicitly

Tasks:

- Record whether persistent loop / daemon work is a separate phase after spec closure.
- Avoid mixing that roadmap into core implementation completion unless intentionally expanded.

### Acceptance criteria

- There is a final, explicit go/no-go view against the main implementation spec.
- The repo has a clean boundary between core coding-agent completion and future daemon/scheduler work.

## 5. File-by-File Working Set

## PR1 working set

- `main.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `internal/agent/runtime_state.go`
- optional new shared runtime-profile/report helper file

## PR2 working set

- `internal/agent/system_prompt.md`
- `internal/agent/agent.go`
- `internal/agent/agent_test.go`
- `internal/ui/commands.go`
- the shared runtime-profile/report helper introduced in PR1

## PR3 working set

- `internal/ui/app.go`
- `internal/ui/app_test.go`
- `internal/ui/state_flow_test.go`
- `internal/ui/workflow_panel.go`
- `internal/ui/workflow_panel_test.go`
- `internal/ui/verification/verification_test.go`
- `internal/agent/router_test.go`

## PR4 working set

- new `internal/eval/` or `eval/` package
- `internal/agent/testdata/`
- `internal/ui/testdata/`
- tests that snapshot runtime prompt/help/workflow surfaces

## PR5 working set

- `docs/implementation-spec.md`
- new acceptance-checklist document if desired
- optional roadmap note for persistent loop work

## 6. Recommended Milestones

## Milestone A — Runtime contract clarity

Includes:

- PR1
- PR2

Outcome:

- startup/runtime behavior is explicit
- model/UI/runtime disclosures are consistent

## Milestone B — Lifecycle confidence

Includes:

- PR3

Outcome:

- UI behavior is well-covered under interruption and error conditions

## Milestone C — Regression framework

Includes:

- PR4

Outcome:

- stable outputs and repo-task behavior are measurable over time

## Milestone D — Spec closure

Includes:

- PR5

Outcome:

- implementation status is auditable against the spec

## 7. Non-Goals for This Plan

This plan does **not** include implementing the persistent loop / daemon system from `docs/persistent-loop-design.md`.

Reason:

- it is a distinct product expansion
- it depends on separate process/IPC/persistence decisions
- it is not required to close the main implementation-spec gaps identified above

If loop/daemon work is prioritized, it should be tracked as a separate roadmap with its own acceptance criteria.

## 8. Definition of Done

This plan is complete when:

1. startup validation is explicit and tested
2. runtime/tool disclosure comes from a shared source of truth
3. lifecycle edge cases are covered by tests
4. eval/golden/run-summary infrastructure exists in-repo
5. `docs/implementation-spec.md` acceptance criteria are re-audited and recorded
6. the repo's CI-equivalent validators pass after each phase:
   - `golangci-lint run`
   - `go test -race ./...`
   - `go vet ./...`
   - `go mod tidy && git diff --exit-code go.mod go.sum`

## 9. Recommended Next Action

Start with **PR1**.

It has the highest leverage because it improves:

- runtime correctness
- operator trust
- diagnostics
- the foundation for PR2 and PR4

# Implementation Spec Acceptance Checklist

This checklist audits the current implementation against `docs/implementation-spec.md` §20.

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

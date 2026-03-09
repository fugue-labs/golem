# Team mode / delegate review

Initial findings:

- Team mode is only enabled when `runtime.EffectiveTeamMode` is true; otherwise the runtime reports it as off and the agent is created without `codetool.WithTeamMode()` (`internal/agent/agent.go:88-90`).
- Delegate mode is hard-disabled whenever `cfg.DisableDelegate` is true; in that case the agent also adds `codetool.WithDisableDelegate()` and team mode is forced off early (`internal/agent/agent.go:85-87`, `internal/agent/router.go:47-49`).
- In `auto` team mode, any router failure conservatively disables team mode and records `auto router unavailable: ...` as the reason (`internal/agent/router.go:56-65`).
- The runtime/tool report only advertises delegate as on when both delegate is allowed and effective team mode is on (`internal/agent/runtime_report.go:93`).
- The repo-defined guaranteed tool surfaces only include repo tools plus workflow tools; teammate/task communication tools are not part of the guaranteed surface at all in this codebase (`internal/agent/runtime_report.go:10-13`, `internal/agent/runtime_report.go:184-203`).

Hypothesis for your current session:

- Your runtime profile showed `team mode: auto (effective: off)` with reason `auto router unavailable: ... Invalid schema for response_format ... additionalProperties ...`.
- That matches the fallback path in `internal/agent/router.go:63-65`: the router call failed, so team mode was turned off conservatively.
- Because delegate is only surfaced when effective team mode is on, delegate also appeared off in the runtime report.
- The teammate communication/task tools do not appear to be wired as first-class surfaces in this repo’s runtime reporting layer, so when team mode is off there is nothing else exposing them.

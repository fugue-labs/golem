# Mission flow audit (historical)

Date: 2026-03-15

This document is preserved as a historical audit snapshot and is **not** the source of truth for the current shipped mission contract.

## Why this file is historical

The audit captured a pre-shipment or earlier implementation state. Since then, the mission flow changed in ways that make several audit findings intentionally outdated, including:

- `Controller.ApplyPlan` now creates a durable mission-plan approval record.
- `/mission approve` resolves that gate through `ApproveMission` and then immediately attempts mission start.
- `StartMission` remains gated until the durable plan approval is approved and any other required approvals are resolved.
- `/mission status` and `golem dashboard` are expected to reflect durable mission state and current Mission Control rendering rather than the older audited assumptions.

## Current sources of truth

Use these documents and code paths for the shipped contract:

- `docs/mission-orchestration-prd.md`
- `docs/features.md`
- `docs/spec-acceptance-checklist.md`
- `internal/mission/controller.go`
- `internal/mission/orchestrator.go`
- `internal/mission/summary.go`
- `internal/ui/mission_commands.go`
- `internal/ui/dashboard/dashboard.go`

## Historical value retained

This file remains useful only as an archival note that an audit was performed on 2026-03-15. If you need the original detailed audit text, inspect git history for that date rather than relying on this file for current behavior.

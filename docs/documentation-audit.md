# Documentation audit: shipped surfaces and source-of-truth map

This audit is the working inventory for documentation updates. It is intentionally grounded in the current code and workflow files, not in aspirational product language.

## How to use this audit

- Treat the **source-of-truth file** in each section as the file docs should follow when wording product behavior.
- Treat `README.md` as a high-level overview, not the final authority, when it differs from code.
- Avoid promoting any behavior called out below as **non-shipped / aspirational**.

## 1. Shipped shell commands

Primary source of truth: `main.go`

| Surface | Shipped behavior | Source of truth | Notes for docs |
| --- | --- | --- | --- |
| `golem` | Launches the main TUI. If extra args are present, they are joined and used as the initial prompt. | `main.go` | This is the default entrypoint. |
| `golem login [provider]` | Starts login flow. Supported explicit providers are `chatgpt`, `anthropic`, `openai`, `xai`. With no provider, shows an interactive picker. | `main.go`, `internal/login/login.go` | Docs should not claim other providers are available through `golem login`. |
| `golem logout` | Removes saved `~/.golem/config.json`, `~/.golem/credentials.json`, and `~/.golem/auth.json`. Does not remove environment variables. | `main.go`, `internal/login/login.go` | Call out that env-based auth remains untouched. |
| `golem dashboard [mission-id]` | Opens the Mission Control dashboard. Optional mission ID selects a specific mission. | `main.go`, `internal/ui/dashboard/dashboard.go` | Treat `main.go` as the entrypoint source and `internal/ui/dashboard/dashboard.go` as the primary dashboard-behavior source; `docs/features.md` is supporting contract context. |
| `golem status [--json]` | Loads config, validates it, prepares runtime, and prints a status summary or JSON report. Exits non-zero on validation/runtime errors. | `main.go`, `internal/agent/runtime_report.go` | Docs should describe this as a one-shot status/config/auth check. |
| `golem runtime [--json]` | Loads config, validates it, prepares runtime, and prints a runtime profile or JSON report. Exits non-zero on validation/runtime errors. | `main.go`, `internal/agent/runtime_report.go` | Docs should describe this as the richer runtime/profile surface. |
| `golem automations` / `golem automations list` | Loads automation config and prints a listing. Default subcommand is `list`. | `main.go` | Defaulting to `list` is shipped behavior. |
| `golem automations start` | Loads automation config and starts the automation daemon until interrupted. | `main.go` | Docs should mention it fails with guidance if no config exists. |
| `golem automations status` | Prints an automation status summary. | `main.go` | Keep claims narrow; this is a status summary, not a full management UI. |
| `golem automations init` | Prints an example `~/.golem/automations.json` config. | `main.go` | This is example generation only, not an interactive setup wizard. |

### Shell-command claims to avoid

- Do **not** document a shipped `golem mission ...` CLI family. `main.go` does not implement it.
- Do **not** document additional `golem automations` subcommands beyond `list|start|status|init`.
- Do **not** document `status`/`runtime` flags beyond `--json`.

## 2. Shipped slash commands inside the TUI

Primary source of truth: `internal/ui/app.go` for top-level dispatch and completion, `internal/ui/commands.go` for general help copy, and `internal/ui/mission_commands.go` for `/mission` subcommand semantics.

### Slash commands currently dispatched

- `/help`
- `/clear`
- `/plan`
- `/invariants`
- `/runtime`
- `/verify`
- `/skills`
- `/skill <name>`
- `/compact`
- `/cost`
- `/budget`
- `/resume`
- `/model [name]`
- `/search <query>`
- `/diff`
- `/undo [path]`
- `/rewind [N]`
- `/doctor`
- `/config`
- `/context`
- `/team`
- `/mission [new|status|tasks|plan|approve|start|pause|cancel|retry [task-id]|list]`
- `/replay [file|list]`
- `/spec [file]`
- `/quit`
- `/exit`

### Discoverability / UX contract that docs may claim

Source of truth: `internal/ui/commands.go`, `internal/ui/app.go`, `internal/ui/mission_commands.go`, `docs/features.md`

- `/help` is the primary in-app discoverability surface. It is safe to document it as the first place users should look for commands and key hints, but not as the canonical complete command registry when dispatch/completion live elsewhere.
- Slash-command tab completion is shipped.
- Unknown slash commands are explicitly handled.
- `/search` without an argument shows usage text including `/search <query>` and examples.
- Dashboard discoverability is part of help copy: `golem dashboard` is the external Mission Control entrypoint.
- `/mission help` and bare `/mission` both expose the shipped mission subcommand list, including `/mission retry [task-id]`.
- Key hints that are safe to document: `Enter`, `Shift+Enter`, `Tab`, `Esc`, `Ctrl+L`, `↑/↓`, `PgUp/PgDn`.

### Slash-command claims to avoid

- Do **not** document `/mission resume`; shipped resume semantics are `/mission start`, not a separate slash command.
- Do **not** document `/mission replan` or task escalation commands as shipped.
- Do **not** document arbitrary slash commands implied by older prose unless they appear in `internal/ui/app.go` dispatch and `slashCommands` completion list.

## 3. Provider and authentication surfaces

Primary source of truth: `internal/login/login.go` for login/logout UX, `internal/config/config.go` for runtime loading/precedence.

### Login entrypoints and saved files

| Surface | Shipped behavior | Source of truth |
| --- | --- | --- |
| ChatGPT login | `golem login chatgpt` uses browser/OAuth login and saves credentials via OpenAI auth helpers. | `internal/login/login.go` |
| API-key login | `golem login anthropic`, `openai`, or `xai` prompts for an API key and saves it to `~/.golem/credentials.json`. | `internal/login/login.go` |
| Provider preference | Successful login saves the selected provider to `~/.golem/config.json`. | `internal/login/login.go` |
| Logout | Removes `config.json`, `credentials.json`, and `auth.json` under `~/.golem/`. | `internal/login/login.go` |

### Provider selection and auth precedence

Source of truth: `internal/config/config.go`

Actual precedence order:

1. `GOLEM_PROVIDER`
2. saved config from `golem login` in `~/.golem/config.json`
3. environment-variable auto-detection / saved key detection
4. default provider: `anthropic`

Provider/auth paths docs may claim:

- `anthropic`
  - auth: `ANTHROPIC_API_KEY` or saved key
  - default model: `claude-sonnet-4-20250514`
- `openai`
  - auth: `OPENAI_API_KEY` or ChatGPT subscription credentials from `~/.golem/auth.json`
  - optional base URL override: `GOLEM_BASE_URL` or `OPENAI_BASE_URL`
  - default model: `gpt-5.4`
- `openai_compatible`
  - runtime path used for xAI/custom compatible endpoints
  - auth: `GOLEM_API_KEY`, `XAI_API_KEY`, or saved `xai` key in `~/.golem/credentials.json`
  - base URL: `GOLEM_BASE_URL`, `XAI_BASE_URL`, or default `https://api.x.ai/v1`
  - default model: `grok-3`
- `vertexai`
  - requires `VERTEX_PROJECT`
  - region from `VERTEX_REGION` defaulting to `us-central1`
  - default model: `gemini-2.5-pro`
- `vertexai_anthropic`
  - requires `VERTEX_PROJECT`
  - region from `VERTEX_REGION` defaulting to `us-central1`
  - default model: `claude-sonnet-4-5`

### Auth/status wording that is safe

Source of truth: `internal/config/config.go`, `internal/agent/runtime_report.go`

- Runtime/status surfaces can describe auth as one of:
  - API key
  - ChatGPT subscription (OAuth)
  - missing / will fail at runtime
- Provider source can be described as one of the surfaced values:
  - `GOLEM_PROVIDER`
  - `golem login`
  - `env`
  - `default`

### Provider/auth claims to avoid

- Do **not** document `golem login` support for Vertex providers; login only supports `chatgpt|anthropic|openai|xai`.
- Do **not** imply ChatGPT login is a generic OpenAI API key flow; it is its own OAuth/subscription path.
- Do **not** imply logout clears environment variables.

## 4. Runtime, config, and reporting surfaces

Primary source of truth: `internal/config/config.go` and `internal/agent/runtime_report.go`

### One-shot reporting surfaces

| Surface | Shipped behavior | Source of truth |
| --- | --- | --- |
| `golem status` | Compact status summary: provider, source, model, auth, base URL, router, timeout, validation, runtime error. | `main.go`, `internal/agent/runtime_report.go` |
| `golem runtime` | Expanded runtime profile with config/runtime/tool-surface details. | `main.go`, `internal/agent/runtime_report.go` |
| `/runtime` | In-app rendering of the same runtime profile family. | `internal/ui/commands.go`, `internal/agent/runtime_report.go` |
| `/config` | In-app rendering of effective config plus selected env vars. | `internal/ui/commands.go` |
| `/doctor` | In-app diagnosis of provider/auth, git repo, working dir, instructions, MCP servers, permission mode, and tool availability checks. | `internal/ui/commands.go` |

### Runtime/profile details docs may claim

Source of truth: `internal/agent/runtime_report.go`

Shipped runtime/profile fields include:

- provider / model
- provider source
- login provider
- auth mode / auth summary
- base URL
- router model and effective router model
- timeout
- team mode and effective team mode
- reasoning effort
- thinking budget
- auto-context max tokens / keep-last turns
- top-level personality flag
- git repo present / branch
- instruction files loaded
- MCP servers / MCP status
- memory on/off
- model routing on/off plus routed model / reason
- runtime errors
- tool surfaces
- validation errors / warnings

### Config/env surfaces docs may claim

Source of truth: `internal/config/config.go`

Important runtime env/config knobs visible in shipped code:

- provider/model/auth/base URL
- `GOLEM_TIMEOUT`
- `GOLEM_ROUTER_MODEL` / `GOLEM_CHEAP_MODEL`
- `GOLEM_TEAM_MODE`
- `GOLEM_DISABLE_DELEGATE`
- `GOLEM_DISABLE_CODE_MODE` / `GOLEM_NO_CODE_MODE`
- `GOLEM_ENABLE_FETCH_URL`
- `GOLEM_TOP_LEVEL_PERSONALITY`
- `GOLEM_DISABLE_GREEDY_THINKING_PRESSURE`
- `GOLEM_PERMISSION_MODE`
- `GOLEM_SESSION_BUDGET`
- `GOLEM_PROJECT_BUDGET`
- `GOLEM_BUDGET_WARN_PCT`
- `GOLEM_FALLBACK_MODEL`
- `GOLEM_BENCHMARK_MODE`
- `GOLEM_PACE_MODE`
- `GOLEM_CHECKPOINT_INTERVAL`
- `GOLEM_PACE_CLARIFY`
- `GOLEM_DISABLED_TOOLS`

### Runtime/config claims to avoid

- Do **not** treat every environment-dependent capability as always present. `internal/agent/runtime_report.go` explicitly says optional capabilities should only be trusted when surfaced by the active runtime/tool list.
- Do **not** document MCP, memory, delegate, execute-code, fetch-url, web-search, or ask-user as unconditionally available.
- Do **not** describe `/config` as a complete dump of every internal knob; it exposes a curated subset.

## 5. Mission and dashboard surfaces

Primary source of truth: `internal/ui/mission_commands.go` for `/mission` command semantics, and `internal/ui/dashboard/dashboard.go` for dashboard behavior. Use `main.go` as the dashboard entrypoint reference and `docs/features.md` as supporting contract context, but not as the primary authority for current dashboard rendering, controls, or empty-state behavior.

### Shipped mission surfaces

| Surface | Shipped behavior | Source of truth |
| --- | --- | --- |
| `/mission new <goal>` | Creates a durable mission in `draft`. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission status` | Shows durable mission summary with status/phase/attention/next action/focus queues and related counts. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission tasks` | Lists durable task DAG details. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission plan` | Runs planning and moves mission toward `awaiting_approval`. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission approve` | Resolves mission-plan approval and attempts execution if no remaining approvals block start. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission start` | Starts a `paused` mission or an already-approved `awaiting_approval` mission. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission pause` | Stops new task leasing by stopping the in-process orchestrator. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission cancel` | Cancels the mission and clears the active mission from the current TUI session. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `/mission retry [task-id]` | Retries a specific failed/blocked task when an ID is given, or retries all failed/blocked tasks when no ID is given. | `internal/ui/mission_commands.go` |
| `/mission list` | Lists known missions and marks the current session's active mission. | `internal/ui/mission_commands.go`, `docs/features.md` |
| `golem dashboard [mission-id]` | Opens Mission Control in a dedicated TUI. Optional mission ID targets a specific mission; otherwise the dashboard auto-selects the highest-priority non-terminal mission, falling back to the most recent mission. | `internal/ui/dashboard/dashboard.go`, `main.go`, `docs/features.md` |

### Dashboard contract docs may claim

Primary source of truth: `internal/ui/dashboard/dashboard.go`

- Dashboard refreshes against durable mission state on load and on a 2-second tick, with manual `r` refresh support.
- With no explicit mission ID, the dashboard selects the most relevant non-terminal mission by priority: `running`, `blocked`, `paused`, `awaiting_approval`, `planning`, then `draft`, then falls back to the most recent mission.
- Mission Control renders four panes: **Tasks**, **Workers**, **Evidence**, and **Events**.
- Dashboard navigation and focus controls are shipped: `Tab`, `Shift+Tab`, `1-4`, `j/k`, arrow keys for scroll aliases, mouse pane focus, `r`, and `q`/`Ctrl+C`.
- The footer and pane headers explicitly teach the pane-navigation contract.
- The header surfaces mission status and progress metrics, including active workers, pending approvals, evidence count, elapsed time, repo, branch, and worker budget.
- Empty state is explicit: the dashboard shows `Mission Control`, `No active mission`, and guidance centered on creating a mission from the main shell with `/mission new`; current copy may still mention `golem mission new` as aspirational wording.
- Error and loading states are explicit user-facing dashboard modes, not silent failures.

Supporting references: `main.go` for CLI routing and `docs/features.md` for broader mission lifecycle context.

### Mission claims to avoid

- Do **not** document a shipped `golem mission ...` shell-command family.
- Do **not** treat the dashboard empty-state phrase `run golem mission new` as proof of a shipped CLI command; this is present in current UI copy but not routed in `main.go`.
- Do **not** document stricter mission repository precondition enforcement as shipped; `docs/features.md` says `CreateMission` does not yet enforce it.
- Do **not** document `/mission replan` or task escalation controls as shipped.
- Do **not** imply a persistent mission daemon is the user-facing execution model; the shipped system is local-first/in-process from the TUI plus durable store-backed status/dashboard surfaces.

## 6. Automation surfaces

Primary source of truth: `main.go`

Shipped docs can claim:

- automation config is loaded from the local Golem config area
- `golem automations list` prints configured automations
- `golem automations init` prints an example `~/.golem/automations.json`
- `golem automations start` starts the daemon until interrupted
- `golem automations status` prints a status summary
- if no automations are configured, `start` prints setup guidance instead of silently succeeding

Claims to avoid:

- no claim of a dedicated automation editor/UI
- no claim of more shell subcommands than `list|start|status|init`
- no claim that automations are configured via login flow

## 7. CI and release surfaces docs may safely mention

Primary source of truth: `.github/workflows/ci.yml` and `.github/workflows/release.yml`

### CI

`ci.yml` currently enforces:

- lint on Ubuntu
- tests on Go `1.26` with `go test -race -coverprofile=coverage.out ./...`
- coverage upload to Codecov
- `go vet ./...`
- `go mod tidy` consistency check on `go.mod` and `go.sum`
- advisory `govulncheck ./...`

### Release

`release.yml` currently enforces:

- release is tag-driven on `v*`
- tag must match semver-like `vMAJOR.MINOR.PATCH` with optional prerelease suffix
- tag major version must match Go module path suffix rules
- release validation runs `go build ./...` and `go test -race ./...`
- GitHub Release notes are generated with `git-cliff`
- release publication uses GitHub Releases, not an external package registry workflow in this file

### CI/release claims to avoid

- Do **not** claim multi-version CI; the workflow currently tests Go `1.26` only.
- Do **not** claim automated package publishing from `release.yml`; this workflow creates a GitHub Release.
- Do **not** claim vulnerability check failures block merges/releases; `govulncheck` is advisory in CI.

## 8. Source-of-truth map for remaining docs work

| Documentation surface | Primary source of truth | Secondary/supporting files |
| --- | --- | --- |
| Shell command reference | `main.go` | `README.md` |
| Slash command reference | `internal/ui/app.go`, `internal/ui/commands.go` | `internal/ui/mission_commands.go`, `docs/features.md` |
| Mission command semantics | `internal/ui/mission_commands.go` | `docs/features.md`, `main.go`, `internal/ui/commands.go` |
| Provider/login docs | `internal/login/login.go` | `internal/config/config.go`, `README.md` |
| Auth/config precedence docs | `internal/config/config.go` | `internal/agent/runtime_report.go` |
| Runtime/status docs | `internal/agent/runtime_report.go` | `main.go`, `internal/ui/commands.go` |
| Mission/dashboard docs | `internal/ui/dashboard/dashboard.go` | `internal/ui/mission_commands.go`, `main.go`, `docs/features.md`, `internal/ui/commands.go` |
| Automation docs | `main.go` | `README.md` |
| CI/release docs | `.github/workflows/ci.yml`, `.github/workflows/release.yml` | `README.md` |
| High-level marketing/overview copy | `README.md` | this audit, `docs/features.md` |

## 9. Recommended documentation guardrails

When updating public docs, prefer these rules:

1. If a command is not routed in `main.go` or `internal/ui/app.go`, do not document it as shipped.
2. If a mission behavior is described as aspirational or future-looking in `docs/features.md`, keep it out of reference docs.
3. If runtime availability depends on the active environment/tool list, document it as conditional.
4. Use `README.md` for orientation and examples, but use code/workflow files for reference-level accuracy.

# Runtime configuration and local state

This guide explains how Golem chooses a provider, where authentication is loaded from, which environment variables affect runtime behavior, what `golem status` and `golem runtime` report, and what lives under `~/.golem/`.

## Provider selection

Golem resolves the active provider in this order:

1. `GOLEM_PROVIDER`
2. Saved login provider in `~/.golem/config.json`
3. Environment-variable auto-detection
4. Saved API keys in `~/.golem/credentials.json` or legacy ChatGPT OAuth credentials in `~/.golem/auth.json`
5. Built-in default: `anthropic`

This precedence comes from `internal/config/config.go`. In code, step 2 is the raw provider recorded by `golem login`, while steps 4-5 are handled by `detectProvider()` when no explicit provider or saved login preference is present.

### Supported provider values

The runtime config currently supports these provider identifiers:

- `anthropic`
- `openai`
- `vertexai`
- `vertexai_anthropic`
- `openai_compatible`

`golem login` exposes these operator-facing login choices:

- `chatgpt` → saved as a login preference, but resolved at runtime as provider `openai`
- `anthropic`
- `openai`
- `xai` → saved as a login preference, but resolved at runtime as provider `openai_compatible`

### Auto-detection rules

If `GOLEM_PROVIDER` is not set and there is no saved login preference, Golem calls `detectProvider()` and checks for the first matching source in this order:

- `ANTHROPIC_API_KEY` → `anthropic`
- `OPENAI_API_KEY` → `openai`
- `XAI_API_KEY`, `GOLEM_BASE_URL`, or `GOLEM_API_KEY` → `openai_compatible`
- `GOOGLE_APPLICATION_CREDENTIALS` or `VERTEX_PROJECT` → `vertexai`
- saved API key `anthropic` in `~/.golem/credentials.json` → `anthropic`
- saved API key `openai` in `~/.golem/credentials.json` → `openai`
- saved API key `xai` in `~/.golem/credentials.json` → `openai_compatible`
- legacy ChatGPT OAuth credentials in `~/.golem/auth.json` with auth mode `chatgpt` → `openai`
- otherwise → `anthropic`

That means saved API keys and legacy ChatGPT OAuth credentials are fallback provider-selection signals only when neither `GOLEM_PROVIDER` nor the saved login provider in `config.json` is present.

## Authentication and credential precedence

Provider selection and authentication are related, but not identical.

### `golem login` persistence

A successful `golem login` writes files under `~/.golem/`:

- `config.json` stores the selected login provider and optional budget settings
- `credentials.json` stores API keys for `anthropic`, `openai`, and `xai`
- `auth.json` stores ChatGPT OAuth credentials used for `golem login chatgpt`

`golem logout` removes `config.json`, `credentials.json`, and `auth.json`. It does **not** clear environment variables.

### Per-provider auth sources

#### Anthropic

- API key source: `ANTHROPIC_API_KEY`, then saved `credentials.json`
- Validation requires a non-empty API key

#### OpenAI

- If ChatGPT OAuth credentials are already active, Golem uses them
- Otherwise it prefers `OPENAI_API_KEY`, then saved `credentials.json`
- If no API key is available, it falls back to ChatGPT OAuth credentials from `~/.golem/auth.json`
- Validation requires either an OpenAI API key or ChatGPT OAuth credentials

#### OpenAI-compatible (`xAI` and custom OpenAI-compatible endpoints)

- API key source: `GOLEM_API_KEY`, then `XAI_API_KEY`, then saved `credentials.json`
- Base URL source: `GOLEM_BASE_URL`, then `XAI_BASE_URL`, then default `https://api.x.ai/v1`
- Validation requires both an API key and a base URL

#### Vertex AI

For both `vertexai` and `vertexai_anthropic`:

- `VERTEX_PROJECT` is required
- `VERTEX_REGION` defaults to `us-central1`
- validation always requires a non-empty project ID
- validation only fails on region if it somehow ends up empty after defaulting

`GOOGLE_APPLICATION_CREDENTIALS` participates in provider auto-detection, but config validation does not require that specific env var. In practice, you still need Google credentials available to the underlying Vertex AI client.

## Key environment variables

The following variables directly affect provider choice, model/auth setup, or runtime reporting.

The lightweight docs QA guardrail in `test/docsqa/docsqa_test.go` intentionally watches a narrow subset of the highest-risk names here: onboarding-critical provider/model variables plus the config keys most likely to break setup docs when they drift.

### Provider and auth

- `GOLEM_PROVIDER`
- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `GOLEM_API_KEY`
- `XAI_API_KEY`
- `GOLEM_BASE_URL`
- `OPENAI_BASE_URL`
- `XAI_BASE_URL`
- `VERTEX_PROJECT`
- `VERTEX_REGION`
- `GOOGLE_APPLICATION_CREDENTIALS`

### Model and timeout

- `GOLEM_MODEL`
- `GOLEM_FAST_MODEL` — overrides the fast model used by model routing before file/default routing config is consulted
- `GOLEM_ROUTER_MODEL`
- `GOLEM_CHEAP_MODEL` — fallback source for `router_model` when `GOLEM_ROUTER_MODEL` is unset
- `GOLEM_TIMEOUT`
- `GOLEM_REASONING_EFFORT`
- `GOLEM_THINKING_BUDGET`
- `GOLEM_FALLBACK_MODEL`

### Team/runtime behavior

- `GOLEM_TEAM_MODE`
- `GOLEM_DISABLE_DELEGATE`
- `GOLEM_DISABLE_CODE_MODE`
- `GOLEM_NO_CODE_MODE`
- `GOLEM_ENABLE_FETCH_URL`
- `GOLEM_TOP_LEVEL_PERSONALITY`
- `GOLEM_DISABLE_GREEDY_THINKING_PRESSURE`
- `GOLEM_PERMISSION_MODE`
- `GOLEM_BENCHMARK_MODE`
- `GOLEM_PACE_MODE`
- `GOLEM_CHECKPOINT_INTERVAL`
- `GOLEM_PACE_CLARIFY`
- `GOLEM_DISABLED_TOOLS`

### Budgeting

These are loaded from `~/.golem/config.json` when present, but env vars override saved values:

- `GOLEM_SESSION_BUDGET`
- `GOLEM_PROJECT_BUDGET`
- `GOLEM_BUDGET_WARN_PCT`
- `GOLEM_FALLBACK_MODEL`

### Optional integrations

- `GOLEM_MCP_SERVERS` — appends MCP server definitions to any servers loaded from `~/.golem/mcp.json`
- `GOLEM_MISSION_DB` — overrides the default mission database path

## Defaults by provider

When a provider is selected, Golem fills in defaults for model and some runtime settings.

| Provider | Default model | Notes |
| --- | --- | --- |
| `anthropic` | `claude-sonnet-4-20250514` | uses `GOLEM_THINKING_BUDGET`, auto-context 150k/12 |
| `openai` | `gpt-5.4` | uses `GOLEM_REASONING_EFFORT`, auto-context 900k/20 |
| `openai_compatible` | `grok-3` | default base URL `https://api.x.ai/v1`, auto-context 900k/20 |
| `vertexai` | `gemini-2.5-pro` | uses `GOLEM_THINKING_BUDGET`, auto-context 900k/20 |
| `vertexai_anthropic` | `claude-sonnet-4-5` | uses `GOLEM_THINKING_BUDGET`, auto-context 150k/12 |

## `golem status` and `golem runtime`

Both commands load config, validate it, prepare runtime state, and then render either text or JSON.

- `golem status` prints a short status summary
- `golem runtime` prints a fuller runtime profile and tool-surface summary
- Successful `golem status --json` and `golem runtime --json` responses are pretty-printed JSON using the same runtime report schema

### JSON behavior

The JSON form includes fields derived from `internal/agent/runtime_report.go`, including:

- provider metadata: `provider`, `provider_source`, `login_provider`, `model`
- auth metadata: `auth_mode`, `auth_summary`, optional `base_url`
- routing/runtime fields such as `router_model`, `effective_router_model`, `timeout`, `team_mode`, `effective_team_mode`, `team_mode_reason`, `reasoning_effort`, `thinking_budget`
- repo/runtime context such as `git_repo`, `git_branch`, `instruction_files`, `mcp_servers`, `mcp_status`, `memory_status`
- model-routing fields: `model_routing`, `routed_model`, `routing_reason`
- `tool_surfaces`
- `validation`
- `runtime_error` when runtime preparation failed

`status --json` and `runtime --json` differ mainly in the non-JSON text renderer. Their JSON payload is built from the same `RuntimeReport` struct.

### Exit codes and failure cases

- If config loading fails and `--json` is used, Golem prints a compact single-object error payload such as `{"error":"..."}` and exits non-zero
- If config loading succeeds, `status --json` and `runtime --json` pretty-print the `RuntimeReport` payload with indentation
- If validation fails, Golem still emits the runtime report, including `validation.errors`, then exits with code 1
- If runtime preparation fails, Golem still emits the report with `runtime_error`, then exits with code 1

That makes `--json` suitable for automation that needs structured output even on invalid or partially available runtimes.

## Reading runtime/tool availability correctly

The runtime report separates always-available repo/workflow tools from optional or environment-dependent surfaces.

### Guaranteed tool lists

`tool_surfaces.repo_tools` and `tool_surfaces.workflow_tools` are filtered only by disabled-tool config. They describe the tools Golem intends to expose from the built-in surface.

Current built-in groups are:

- repo tools: `bash`, `bash_status`, `bash_kill`, `view`, `edit`, `write`, `multi_edit`, `glob`, `grep`, `ls`, `lsp`
- workflow tools: `planning`, `invariants`, `verification`

By default, Golem disables a lean subset of tools via `GOLEM_DISABLED_TOOLS` defaults. The default disabled set is:

- `lsp`
- `multi_edit`
- `verification`
- `invariants`
- `open_image`
- `delegate`
- `execute_code`

Set `GOLEM_DISABLED_TOOLS=none` to remove that default disabled set, or provide a comma-separated list to disable a custom set.

### Environment-dependent surfaces

These fields should be treated as runtime facts, not documentation promises:

- `delegate`
- `execute_code`
- `open_image`
- `web_search`
- `fetch_url`
- `ask_user`

This is also called out directly in the runtime report via:

> Environment-dependent capabilities should only be trusted when surfaced by the active runtime/tool list.

In practice:

- `delegate` is `on` only when team mode is effectively on, delegate is not disabled, and the tool is not disabled
- `execute_code` can be `on`, `off`, `pending`, or `unavailable` depending on config and local runtime support
- `open_image` is `on` only when the resolved model supports vision and the tool is not disabled; otherwise it is `off` or `pending`
- `web_search` is currently reported as `off`
- `fetch_url` is `on` only when `GOLEM_ENABLE_FETCH_URL` is truthy
- `ask_user` depends on team-mode behavior and may be `off`, `pending`, or `on`

### Other runtime-dependent fields

A few non-tool fields are also environment-dependent and should be interpreted from the active report, not assumed statically:

- `git_repo` / `git_branch`
- `instruction_files`
- `mcp_status` / `mcp_servers`
- `memory_status`
- `model_routing` / `routed_model` / `routing_reason`

## Local state under `~/.golem/`

The following paths are currently documented or written by the codebase.

### Core config and auth

- `~/.golem/config.json` — saved login provider plus optional saved budget settings
- `~/.golem/credentials.json` — saved API keys for login-based providers
- `~/.golem/auth.json` — ChatGPT OAuth credentials

### Runtime and agent behavior

- `~/.golem/instructions.md` — user-level instruction file discovered before project-local instruction files
- `~/.golem/hooks.json` — pre/post tool shell hooks
- `~/.golem/mcp.json` — MCP server definitions
- `~/.golem/routing.json` — model-routing configuration

### Persistent agent data

- `~/.golem/sessions/<project-hash>/` — saved session JSON files for replay/resume/search
- `~/.golem/memory/<project-hash>/memory.db` — persistent per-project memory SQLite database
- `~/.golem/missions.db` — default mission database, unless overridden by `GOLEM_MISSION_DB`

### Automations

- `~/.golem/automations.json` — automation configuration
- `~/.golem/automations/daemon.pid` — daemon PID file for `golem automations status`
- `~/.golem/automations/logs/` — log files for automation-triggered runs

## Operator notes

- Prefer `GOLEM_PROVIDER` when you need an explicit, one-off override
- Prefer `golem login` when you want a sticky default provider for future runs
- Use `golem status --json` or `golem runtime --json` for scripts, dashboards, and health checks
- When checking tool availability, trust the active runtime report over assumptions from docs or environment alone

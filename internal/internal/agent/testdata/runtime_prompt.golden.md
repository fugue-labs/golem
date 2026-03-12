# Golem Runtime Profile

## Effective runtime
- Provider/model: `openai/gpt-5.4`
- Provider source: `GOLEM_PROVIDER`
- Router model: `router-mini`
- Effective router model: `router-resolved`
- Timeout: `1m0s`
- Team mode: `auto` (effective: `off`)
- Team mode reason: auto router pending
- Reasoning effort: `low`
- Auto-context: `100` tokens, keep last `2` turns
- Top-level personality: `true`
- Git repo: `off`

## Tool surfaces
- Guaranteed repo tools: `bash`, `bash_status`, `bash_kill`, `view`, `edit`, `write`, `multi_edit`, `glob`, `grep`, `ls`, `lsp`
- Guaranteed workflow tools: `planning`, `invariants`, `verification`
- Delegate: `off`
- Execute code: `on`
- Open image: `off`
- Web search: `off`
- Fetch URL: `off`
- Ask user: `pending`
- Environment-dependent capabilities should only be trusted when surfaced by the active runtime/tool list.

# Getting started with Golem

Golem is a terminal-native coding agent for real software projects. This guide covers the supported path from a fresh checkout to your first interactive session, including install options, provider-specific authentication, environment-variable setup, and the first commands to run inside the TUI.

## What you need

- Go **1.26+**
- Credentials for at least one supported provider

Golem supports two main onboarding styles:

- **Saved login** with `golem login` for ChatGPT, Anthropic, OpenAI, or xAI
- **Environment variables** for ephemeral, CI, or provider setups that are not handled by `golem login`, including Vertex AI and custom OpenAI-compatible endpoints

## Install Golem

From the repository root, pick one of these options.

### Build from source in the current checkout

```bash
go build -o golem .
```

Run the local binary directly:

```bash
./golem
```

### Install with `go install`

```bash
go install github.com/fugue-labs/golem@latest
```

Make sure your Go bin directory is on `PATH` so the `golem` command is available.

## Choose a provider and authenticate

### Option 1: interactive `golem login`

Run `golem login` to pick from the built-in providers:

```bash
golem login
```

Today that picker supports:

- `chatgpt` — ChatGPT subscription login in a browser via OAuth
- `anthropic` — API key
- `openai` — API key
- `xai` — API key

You can also skip the picker and go straight to a provider:

```bash
golem login chatgpt
golem login anthropic
golem login openai
golem login xai
```

### What each login path does

#### ChatGPT (`golem login chatgpt`)

- Opens a browser-based OAuth flow
- Saves ChatGPT subscription credentials to `~/.golem/auth.json`
- Saves your provider preference to `~/.golem/config.json`

ChatGPT login is not an API-key flow. Internally, Golem runs this as the OpenAI provider with ChatGPT OAuth credentials, so `golem status` and `golem runtime` show an OpenAI provider plus ChatGPT subscription auth.

#### Anthropic, OpenAI, and xAI (`golem login anthropic|openai|xai`)

- Prompt for an API key in the terminal
- Save the key to `~/.golem/credentials.json`
- Save your provider preference to `~/.golem/config.json`

### Option 2: environment-variable auth

If you do not want to save credentials locally, Golem can be configured directly from your shell environment.

### Anthropic

```bash
export ANTHROPIC_API_KEY="your-key"
```

### OpenAI API key

```bash
export OPENAI_API_KEY="your-key"
```

### xAI / Grok

```bash
export XAI_API_KEY="your-key"
```

By default, the xAI path uses the built-in xAI base URL. To override it after selecting the provider, set `XAI_BASE_URL` or `GOLEM_BASE_URL`.

### Custom OpenAI-compatible endpoint

```bash
export GOLEM_PROVIDER=openai_compatible
export GOLEM_API_KEY="your-key"
export GOLEM_BASE_URL="https://your-endpoint.example/v1"
```

### Vertex AI

Vertex AI is configured through environment variables rather than `golem login`:

```bash
export GOLEM_PROVIDER=vertexai
export VERTEX_PROJECT="your-gcp-project"
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/.config/gcloud/application_default_credentials.json"
# optional: defaults to us-central1 if unset
export VERTEX_REGION="us-central1"
```

`VERTEX_PROJECT` is required for Vertex providers. If `VERTEX_REGION` is unset, Golem defaults it to `us-central1`.

### Vertex AI with Anthropic models

```bash
export GOLEM_PROVIDER=vertexai_anthropic
export VERTEX_PROJECT="your-gcp-project"
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/.config/gcloud/application_default_credentials.json"
# optional: defaults to us-central1 if unset
export VERTEX_REGION="us-central1"
```

## How Golem chooses provider and auth

For the main onboarding path, provider selection precedence is:

1. `GOLEM_PROVIDER`
2. Saved provider preference from `golem login` in `~/.golem/config.json`
3. Environment-variable auto-detection
4. Default provider: `anthropic`

That is the behavior most users will see because `golem login` writes `config.json`.

For the full implementation behavior, if `GOLEM_PROVIDER` is unset and `~/.golem/config.json` is missing, Golem continues checking in this order:

1. Environment-variable auto-detection
   - `ANTHROPIC_API_KEY`
   - `OPENAI_API_KEY`
   - `XAI_API_KEY`, `GOLEM_BASE_URL`, or `GOLEM_API_KEY`
   - `GOOGLE_APPLICATION_CREDENTIALS` or `VERTEX_PROJECT`
2. Saved API keys in `~/.golem/credentials.json`
3. Legacy ChatGPT OAuth credentials in `~/.golem/auth.json`
4. Fallback default: `anthropic`

A few auth details are useful to know:

- If you explicitly log in with `golem login chatgpt`, Golem uses ChatGPT OAuth credentials from `auth.json` and records `chatgpt` as the saved provider.
- If OpenAI mode is selected without ChatGPT being the saved login, Golem prefers `OPENAI_API_KEY` or a saved OpenAI API key.
- If OpenAI mode has no API key available, Golem can still fall back to saved ChatGPT OAuth credentials from `auth.json`.
- `GOLEM_PROVIDER` always wins over any saved login state.

## Check your setup before launching

These commands are useful before opening the TUI:

```bash
golem status
golem runtime
golem runtime --json
```

- `golem status` prints a compact provider and auth summary
- `golem runtime` prints a fuller runtime profile
- `golem runtime --json` prints the same information in machine-readable form

If you logged in with ChatGPT, expect auth to read as ChatGPT subscription OAuth. If you are using API keys, expect auth to read as API key.

## Start your first interactive session

Launch the app:

```bash
golem
```

Or start with an initial prompt:

```bash
golem fix the failing tests
```

## First commands to try inside Golem

Once the TUI opens, run these in order:

```text
/help
/runtime
/doctor
```

What they do:

- `/help` shows the command list and keybindings
- `/runtime` shows the effective runtime configuration from inside the app
- `/doctor` helps diagnose setup issues if authentication, config, or tooling are missing

A good first prompt after that is something simple like:

```text
summarize this repository
```

Other useful early commands:

- `/search <query>`
- `/plan`
- `/cost`
- `/mission new <goal>`

## Log out or switch providers later

To remove saved login state:

```bash
golem logout
```

This removes saved local files:

- `~/.golem/config.json`
- `~/.golem/credentials.json`
- `~/.golem/auth.json`

It does **not** remove any environment variables from your shell.

## Troubleshooting

- If Golem selects the wrong provider, check `GOLEM_PROVIDER` first.
- If a previous `golem login` keeps winning over your environment variables, either set `GOLEM_PROVIDER` explicitly or run `golem logout`.
- If startup fails, run `golem status` or `golem runtime` to inspect the provider source and auth mode.
- Inside the TUI, run `/doctor` for interactive diagnostics.

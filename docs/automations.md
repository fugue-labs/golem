# Automations

Automations are the advanced, long-running shell workflow in Golem. They are different from the normal interactive `golem` TUI:

- **Beginner / interactive usage**: run `golem`, chat in the TUI, and use slash commands like `/help`.
- **Advanced / long-running usage**: configure automations under your local Golem config area and manage them with `golem automations ...`.

This guide documents only the shell behavior that is currently shipped.

If you still need installation or authentication help, start with [Getting started with Golem](getting-started.md).

## What is shipped

The shipped automation command family is:

```bash
golem automations
golem automations list
golem automations start
golem automations status
golem automations init
```

There are no other documented automation subcommands in the current CLI.

## Beginner path vs advanced path

### Beginner path

If you are new to Golem, start here:

```bash
golem
```

Then use:

```text
/help
/runtime
/doctor
```

You do not need automations for normal day-to-day interactive work.

### Advanced path

Use automations when you want a configuration-backed workflow that runs from the shell rather than from the chat UI.

The current CLI expects automation configuration in your local Golem area. The example config path surfaced by the CLI is:

```text
~/.golem/automations.json
```

## `golem automations`

Running the command without a subcommand defaults to **`list`**:

```bash
golem automations
```

This behaves the same as:

```bash
golem automations list
```

## `golem automations list`

Use `list` to print the configured automations:

```bash
golem automations list
```

This is the safest first command because it tells you what the current configuration contains.

## `golem automations init`

Use `init` to print an example configuration:

```bash
golem automations init
```

The CLI prints:

- a heading for an example `~/.golem/automations.json`, and
- the example configuration content itself.

This is example generation only. It is not an interactive setup wizard and it does not write the file for you.

## `golem automations start`

Use `start` to launch the automation daemon from the current configuration:

```bash
golem automations start
```

The shipped behavior is intentionally narrow:

- Golem loads the automation config.
- If configuration exists, it starts the daemon and keeps running until interrupted.
- If no automations are configured, it exits with setup guidance instead of silently succeeding.

When no config is present, the CLI points you back to the local config path and suggests using `golem automations init`.

### Operator expectation

`start` is the command for an ongoing automation process, not a one-shot status check. Run it when you want the configured daemon to be active, and interrupt it when you want it to stop.

## `golem automations status`

Use `status` for a status summary:

```bash
golem automations status
```

This is a summary surface, not a full automation management UI.

### Operator expectation

If you want a quick human-readable check, use `status`. If you want to actually run the daemon, use `start`.

## Recommended first steps

If you want to explore automations without overcommitting, use this order:

1. Print the example config:

   ```bash
   golem automations init
   ```

2. Review your configured entries:

   ```bash
   golem automations list
   ```

3. Check the summary surface:

   ```bash
   golem automations status
   ```

4. Start the daemon when you are ready for a long-running process:

   ```bash
   golem automations start
   ```

## What this guide does not assume

To stay aligned with the shipped CLI, this guide does **not** assume:

- an automation editor inside the app,
- extra `golem automations` subcommands beyond `list`, `start`, `status`, and `init`, or
- an interactive login-based automation setup flow.

In short: beginners can ignore automations and work entirely in the main TUI, while advanced operators can use `golem automations ...` when they need a config-backed, long-running shell workflow.

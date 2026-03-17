# Automations

Automations are Golem’s advanced, configuration-backed shell workflow. They are different from the normal interactive `golem` TUI:

- **Beginner / interactive usage**: run `golem`, work in the TUI, and use slash commands like `/help`, `/runtime`, and `/doctor`.
- **Advanced / long-running usage**: manage automation configuration under your local Golem area and operate it with `golem automations ...`.

This guide documents only the automation commands that are currently shipped in the CLI.

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

There are no other shipped automation subcommands documented by this guide.

## Beginner path vs advanced path

### Beginner path

If you are new to Golem, start with the main app:

```bash
golem
```

Useful first commands:

```text
/help
/runtime
/doctor
```

You do not need automations for normal day-to-day interactive use.

### Advanced path

Use automations when you want a local config file to define long-running shell behavior outside the main chat UI.

The shipped CLI looks for automation configuration at:

```text
~/.golem/automations.json
```

## `golem automations`

Running `golem automations` without a subcommand defaults to **`list`**:

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

What to expect:

- If configuration is present and has entries, Golem prints a tabular summary with the automation name, trigger type, enabled state, and details.
- If the config file is missing, or the file exists but contains zero automations, Golem prints a human-readable message saying no automations are configured and points you to `~/.golem/automations.json`.
- If the config file cannot be read or parsed, the command exits with an error.

`list` is the safest first command because it tells you what the current configuration contains without attempting to start the daemon.

## `golem automations init`

Use `init` to print an example configuration:

```bash
golem automations init
```

The CLI prints:

- a heading for an example `~/.golem/automations.json`, and
- the example JSON itself.

`init` does not write the file for you. It is example output, not an interactive setup wizard.

## `golem automations start`

Use `start` to launch the automations daemon from the current configuration:

```bash
golem automations start
```

The shipped behavior is precise:

1. Golem loads `~/.golem/automations.json`.
2. If the file is missing, the CLI exits with setup guidance instead of starting:
   - it prints `no automations configured`,
   - tells you to create `~/.golem/automations.json`, and
   - suggests `golem automations init` for an example configuration.
3. If the file exists, Golem attempts to start the daemon with that config.
4. If daemon startup fails, the command exits with an error from daemon startup.

That last point matters:

- a **present-but-empty** config does **not** count as a successful start; daemon startup fails with `no automations configured`, and
- a **present-but-invalid** config also exits with a daemon startup error such as invalid configuration or trigger validation failures.

If startup succeeds, `start` becomes the long-running automation process and continues until interrupted.

### Operator expectation

`start` is not a dry run or a one-shot check. Use it when you want the configured daemon to be active. If you only want to inspect config or daemon state, use `list` or `status` instead.

## `golem automations status`

Use `status` for a quick daemon summary:

```bash
golem automations status
```

The shipped output is intentionally simple:

- `Automations daemon: not running`, or
- `Automations daemon: running (PID <pid>)`

This is a status surface, not a full automation management UI.

## Recommended first steps

If you want to explore automations without overcommitting, use this order:

1. Print the example config:

   ```bash
   golem automations init
   ```

2. Create or review `~/.golem/automations.json`.

3. Inspect the configured entries:

   ```bash
   golem automations list
   ```

4. Check whether a daemon is already running:

   ```bash
   golem automations status
   ```

5. Start the daemon when you are ready for a long-running process:

   ```bash
   golem automations start
   ```

## What this guide does not assume

To stay aligned with the shipped CLI, this guide does **not** assume:

- an in-app automation editor,
- extra `golem automations` subcommands beyond `list`, `start`, `status`, and `init`, or
- a setup flow that silently creates config files for you.

In short: beginners can ignore automations and work entirely in the main TUI, while advanced operators can use `golem automations ...` when they need a config-backed, long-running shell workflow.

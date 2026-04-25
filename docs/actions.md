# Actions

Actions are the script-runner half of the notification subsystem. A
`notify:` rule on an event or root may reference a named action; when
the rule fires, the daemon spawns the action's command with curated
event metadata in the environment.

The actions file is **local to the laptop**. It is **not** stored in the
synced vault. This split is the trust boundary — see [the threat
model](security-reviews/notif-threat-model-2026-04-25.md) for the full
reasoning. The short version: if the synced vault could declare script
paths directly, a compromised sync peer (phone, tablet, replica) becomes
a remote-code-execution primitive on the laptop. Decoupling action
*names* (vault, synced) from action *commands* (laptop, unsynced) means
the worst the vault can do is fire a wrong-time action you already
blessed.

## Where the file lives

```sh
calemdar notify actions    # list actions registered in the file
```

By default cale**md**ar reads from `~/.config/calemdar/actions.yaml`.
You can override the path via `notifications.actions.config_path` in the
main config.

The file is optional — when missing, the runner returns an empty
registry and any rule that names an action is logged and skipped.

## Format

```yaml
actions:
  pre-meeting:
    cmd: /home/me/bin/join-meeting.sh
    timeout: 10s

  morning-bell:
    cmd: ["/usr/bin/notify-send", "-u", "low", "morning"]

  pomo-log:
    shell: "echo done >> ~/.local/state/pomo.log"
```

### Per-action keys

- `cmd` — either a single path string or an array of strings (`argv`).
  Spawned via direct `exec`, no shell. Shell metacharacters in either
  form are passed literally to the program.
- `shell` — a string passed to `sh -c`. Use this when you need pipes,
  redirects, or other shell features. Mutually exclusive with `cmd`.
- `timeout` — Go duration string (`30s`, `2m`). Default 30s. Long-running
  actions are killed when this expires.

Exactly one of `cmd` or `shell` must be set.

### Action names

Action names match `^[a-z][a-z0-9-]{0,47}$` — lowercase, alpha-first,
hyphens permitted, max 48 chars. The same regex is enforced on the vault
side, so what one half accepts the other half accepts.

## Environment passed to the script

The runner does **not** inherit the daemon's environment. The child sees:

- `PATH=/usr/local/bin:/usr/bin:/bin`
- `HOME` (from the parent)
- `USER` (from the parent)
- `CALEMDAR_TITLE` — event title
- `CALEMDAR_DATE` — event date (`YYYY-MM-DD`)
- `CALEMDAR_START` — event start time (`HH:MM`)
- `CALEMDAR_END` — event end time (`HH:MM`, may be empty)
- `CALEMDAR_PATH` — absolute path to the event markdown file
- `CALEMDAR_CALENDAR` — the events/<calendar>/ folder name
- `CALEMDAR_LEAD` — the lead string from the rule (`5m`, `0`, …)

This blocks accidental leakage of daemon-side secrets (e.g. a
`NTFY_TOKEN` env var) into action subprocesses.

## Wiring it to a notify rule

In any event or recurring root frontmatter:

```yaml
notify:
  - lead: 5m
    via: [system]      # also fire a desktop notification
    action: pre-meeting

  - lead: 0
    action: pre-meeting
```

When the rule fires:

1. The scheduler dispatches the message to every backend in `via`.
2. If `action:` is set, the named action is spawned with the env above.
3. The fire is recorded in `notify_fired` so a daemon restart doesn't
   replay it.

## Concurrency

`notifications.max_concurrent_spawns` (default 4) caps how many actions
can run at once. Beyond that they queue.

## Testing

`calemdar notify actions` lists what's registered. To dry-run a fire,
trigger an event with a `notify: [{lead: 0, action: yourname}]` rule and
watch the daemon log.

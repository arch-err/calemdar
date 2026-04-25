# Configuration

cale**md**ar reads a YAML config from the XDG config directory, defaulting
to `~/.config/calemdar/config.yaml`. Every field is optional; missing keys
fall back to the defaults documented below.

## Where the file lives

```sh
calemdar config path     # print the lookup path
calemdar config show     # print the active (post-merge) values
calemdar config init     # write a default config (errors if one exists)
calemdar config edit     # open in $EDITOR, validate on save
```

Resolution order, highest wins:

1. `--vault` flag (vault path only)
2. `$CALEMDAR_VAULT` environment variable (vault path only)
3. `vault:` key in the config file
4. Built-in defaults (for non-vault keys)

## Keys

### `vault`

**Type:** string (path)
**Default:** none — required for the daemon

Absolute path to your Obsidian vault. `~/` is expanded.

```yaml
vault: /home/you/obsidian/my-vault
```

You can also leave this empty in the file and supply it via `--vault` or
`$CALEMDAR_VAULT`.

### `base_path`

**Type:** string (relative path)
**Default:** `""` (vault root)

Optional subfolder inside the vault under which cale**md**ar's tree lives.
Empty means directly under the vault root. Useful when your vault already
has top-level structure and you want calendar data tucked away.

```yaml
base_path: "02 - Personal/Calendar"
```

Must stay inside the vault; `..` segments are rejected.

### `timezone`

**Type:** IANA timezone string
**Default:** `Europe/Stockholm`

Used for "today" resolution and the nightly timer. v1 is effectively
single-timezone — every event is stored in this zone and there is no per-event
override.

```yaml
timezone: Europe/Stockholm
```

### `nightly_at`

**Type:** `HH:MM` string (24h, local to `timezone`)
**Default:** `03:00`

When the nightly reconcile + archive pass runs.

```yaml
nightly_at: "03:00"
```

### `horizon_months`

**Type:** integer (1–120)
**Default:** `12`

How many months ahead of today to keep events materialised. The nightly
timer extends this window as days pass.

```yaml
horizon_months: 12
```

### `archive_cutoff_months`

**Type:** integer (0–120)
**Default:** `6`

Events older than `(today - this many months)` are moved into `archive/`
during the nightly pass. Set to `0` to archive everything past.

```yaml
archive_cutoff_months: 6
```

### `debounce_ms`

**Type:** integer (1–60000)
**Default:** `500`

Filesystem-event coalesce window in milliseconds. Multiple writes to the
same path within the window collapse into a single reconcile. Longer values
are more efficient; shorter values respond faster.

```yaml
debounce_ms: 500
```

### `calendars`

**Type:** list of strings
**Default:** `[health, tech, work, life, friends-family, special]`

The calendar subfolders under `events/`. Each is a Full Calendar source
with its own colour. Adding to the list requires a Full Calendar settings
update and usually a re-scaffold (`calemdar setup`).

```yaml
calendars:
  - health
  - tech
  - work
  - life
  - friends-family
  - special
```

### `notifications`

Per-event notifications. Each event (or recurring root) declares its
own `notify:` rules in the markdown frontmatter — see [Schema](schema.md)
— and the daemon dispatches them through one or more **backends**
(currently `system` for libnotify-via-`notify-send`, and `ntfy` for
ntfy.sh / self-hosted ntfy). A rule may also reference a named **action**
that runs a local script when fired.

```yaml
notifications:
  enabled: true
  tick_interval: 1m
  max_lead: 23h
  max_concurrent_spawns: 4
  calendars: []           # empty = all calendars

  backends:
    system:
      enabled: true
      # binary_path: /usr/bin/notify-send   # override the default lookup
      # urgency: low                         # low | normal | critical
    ntfy:
      enabled: true
      url: https://ntfy.sh
      topic: my-private-calemdar-topic

  actions:
    enabled: false                            # opt-in
    # config_path: ~/.config/calemdar/actions.yaml
```

#### `notifications.enabled`

**Type:** boolean
**Default:** `false`

Master switch. With this off, the daemon never runs the scheduler — no
backend is registered, no action runs, no `notify:` rule fires.

#### `notifications.tick_interval`

**Type:** duration string (`30s`, `1m`, `5m`)
**Default:** `1m`

How often the scheduler wakes up. Per-rule fire times are quantised to
this interval, so a `lead: 5m` rule fires within `tick_interval` of the
intended moment. Values below `30s` are rejected.

#### `notifications.max_lead`

**Type:** duration string (`5m`, `1h`, `23h`)
**Default:** `23h`

Caps the longest per-rule lead the scheduler will scan for. The cap
exists so the lookahead query stays scoped to roughly today's events.
Hard ceiling: 24h.

#### `notifications.max_concurrent_spawns`

**Type:** integer
**Default:** `4`

Caps the action runner's parallelism. A flurry of fires that all carry
an action will queue at this depth.

#### `notifications.calendars`

**Type:** list of strings
**Default:** `[]` (all calendars)

Restrict the scheduler to events in these calendars. Empty means all.

#### `notifications.backends.system`

The desktop-notification backend. Shells out to `notify-send` with the
event title and a short body line.

- `enabled` — boolean. Off by default.
- `binary_path` — override the `notify-send` binary lookup.
- `urgency` — `low` | `normal` | `critical`. Empty leaves `notify-send`'s
  default.

#### `notifications.backends.ntfy`

The ntfy backend. POSTs to `<url>/<topic>` with the event title in the
body and tags in the headers.

- `enabled` — boolean. Off by default.
- `url` — base URL (e.g. `https://ntfy.sh`).
- `topic` — topic name. Keep it unguessable on public servers.

#### `notifications.actions`

Wires the script-runner side. Disabled by default — vault frontmatter
cannot fire scripts unless you opt in.

- `enabled` — boolean. Off by default.
- `config_path` — override the actions file location. Empty falls back
  to `~/.config/calemdar/actions.yaml` (XDG-aware).

See [Actions](actions.md) for the actions.yaml format and the trust
model.

You can verify backend wiring end-to-end without waiting for a real
event:

```sh
calemdar notify test          # fires a test through every enabled backend
calemdar notify test ntfy     # restrict to one backend
calemdar notify actions       # list registered actions
```

## Full example

```yaml
vault: /home/you/obsidian/my-vault
base_path: ""
timezone: Europe/Stockholm
nightly_at: "03:00"
horizon_months: 12
archive_cutoff_months: 6
debounce_ms: 500
calendars:
  - health
  - tech
  - work
  - life
  - friends-family
  - special
notifications:
  enabled: false
  tick_interval: 1m
  max_lead: 23h
  max_concurrent_spawns: 4
  backends:
    system:
      enabled: false
    ntfy:
      enabled: false
      # url: https://ntfy.sh
      # topic: my-private-calemdar-topic
  actions:
    enabled: false
```

See also [`examples/config.yaml`](https://github.com/arch-err/calemdar/blob/main/examples/config.yaml)
in the repo.

## Validation

`calemdar config edit` re-validates on save. Any invalid field prints a
diagnostic and leaves `Active` unchanged so the daemon keeps the last good
values. You can also validate any time with:

```sh
calemdar config show    # prints the merged config or the parse error
```


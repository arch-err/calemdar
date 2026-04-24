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

Upcoming-event push notifications via [ntfy](https://ntfy.sh). When
enabled, the `serve` daemon runs a 60-second ticker that queries the
store for upcoming events and POSTs to `<ntfy_url>/<ntfy_topic>` when a
start time crosses one of the configured lead-minute windows. All-day
events are skipped (no natural trigger time).

```yaml
notifications:
  enabled: true
  ntfy_url: https://ntfy.sh
  ntfy_topic: my-private-calemdar-topic
  lead_minutes: [5, 60]   # default
  calendars: []           # empty = all calendars
```

#### `notifications.enabled`

**Type:** boolean
**Default:** `false`

Switch. When `false`, the daemon does nothing ntfy-related regardless of
the other fields.

#### `notifications.ntfy_url`

**Type:** string (URL)
**Default:** none — required when `enabled`

Base URL of the ntfy server. Public `https://ntfy.sh` works; self-hosted
instances work too.

#### `notifications.ntfy_topic`

**Type:** string
**Default:** none — required when `enabled`

Topic name appended to `ntfy_url`. Keep it unguessable if the server is
public — anyone with the topic name can read and write.

#### `notifications.lead_minutes`

**Type:** list of positive integers
**Default:** `[5, 60]`

Minutes before an event to push. Each value produces its own notification.
`[5, 60]` pushes once an hour out and once five minutes out.

#### `notifications.calendars`

**Type:** list of strings
**Default:** `[]` (all calendars)

Restrict notifications to events in these calendars. Empty means all.

You can verify configuration end-to-end without waiting for a real event:

```sh
calemdar notify test
```

This POSTs a single test push and exits. It does NOT gate on
`enabled`, so you can confirm URL + topic before flipping the switch on
the daemon.

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
  # ntfy_url: https://ntfy.sh
  # ntfy_topic: my-private-calemdar-topic
  # lead_minutes: [5, 60]
  # calendars: []
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


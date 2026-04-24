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

## Notifications (coming)

A future `notifications:` section will configure the `calemdar-notify`
sidecar (ntfy push on upcoming events). Not wired in v1 — see the
[FAQ](faq.md#when-do-notifications-land) and
[design](design.md#notifications-deferred) for the intended shape.

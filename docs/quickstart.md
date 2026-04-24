# Quickstart

Zero to running daemon in five commands.

## Prerequisites

- Go 1.22 or newer on your path.
- An Obsidian vault you own (local disk; Syncthing-synced is fine).
- The [Full Calendar plugin](https://github.com/obsidian-community/obsidian-full-calendar)
  installed in that vault.

## 1. Install the binary

```sh
go install github.com/arch-err/calemdar/cmd/calemdar@latest
```

This drops `calemdar` into `$GOPATH/bin` (usually `~/go/bin`). Make sure that
directory is on your `$PATH`.

Or build from source:

```sh
git clone https://github.com/arch-err/calemdar.git
cd calemdar
just install    # or: go install ./cmd/calemdar
```

AUR and Homebrew packages are not yet available. See the
[FAQ](faq.md#can-i-install-via-aur-homebrew).

## 2. Write the config

```sh
calemdar config init
```

This writes `~/.config/calemdar/config.yaml` with all defaults and drops you
into `$EDITOR`. At minimum, set `vault:` to your Obsidian vault path:

```yaml
vault: /home/you/obsidian/my-vault
```

Everything else has sane defaults. See [Configuration](configuration.md) for
the full key list.

## 3. Scaffold the vault folders

```sh
calemdar setup
```

Creates `recurring/`, `archive/`, and `events/<calendar>/` under the vault
(or under `base_path` if set). Idempotent — safe to re-run. The daemon also
runs this at startup.

## 4. Configure Full Calendar sources

In Obsidian, open Full Calendar settings and add one **Local Calendar**
source per calendar subfolder under `events/`. Defaults are:

- `events/health`
- `events/tech`
- `events/work`
- `events/life`
- `events/friends-family`
- `events/special`

Give each a distinct colour. Do NOT add `recurring/` or `archive/` as
sources — the daemon owns those.

## 5. Start the daemon

### As a systemd user service (recommended)

```sh
cp examples/calemdar.service ~/.config/systemd/user/calemdar.service
systemctl --user daemon-reload
systemctl --user enable --now calemdar.service
journalctl --user -u calemdar -f    # watch it work
```

All tuning lives in `config.yaml`; the unit file takes no env or flags.

### Or run it in a terminal

```sh
calemdar serve
```

Ctrl-C to stop.

## First event

Create a recurring series interactively:

```sh
calemdar series new
```

cale**md**ar writes the root to `recurring/<slug>.md`, then immediately
expands it into concrete events under `events/<calendar>/`. Open Obsidian's
Full Calendar and they appear on the grid.

To test the drag-safety: grab any expanded occurrence in Full Calendar and
drop it on a new time. Open the file — `user-owned: true` is now in the
frontmatter. The rest of the series is untouched, and future regenerations
will leave this occurrence alone.

You can also create one-offs without a recurrence rule:

```sh
calemdar event new
```

## Next

- [CLI reference](cli.md) for every subcommand.
- [Architecture](architecture.md) for how reconcile, watcher, and store fit
  together.
- [FAQ](faq.md) for things that might trip you up.

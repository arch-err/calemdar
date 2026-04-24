<p align="center">
  <img src="assets/logo.svg" alt="calemdar" width="160">
</p>

<h1 align="center">cale<b>md</b>ar</h1>

<p align="center"><em>Recurring-event manager for Obsidian Full Calendar. Markdown as source of truth, server-expanded occurrences.</em></p>

<!-- BADGES -->

---

Obsidian's [Full Calendar plugin](https://github.com/obsidian-community/obsidian-full-calendar)
has a long-standing footgun: drag one occurrence of a recurring event to
reschedule it and the plugin overwrites the recurrence rule, wiping every
other occurrence in the series. cale<b>md</b>ar sidesteps it by keeping
recurrence rules in server-private roots and materialising each occurrence as
an independent flat event file. Full Calendar only ever sees single events,
so drag, drop, and edit are safe.

## Install

```sh
go install github.com/arch-err/calemdar/cmd/calemdar@latest
```

Or build from source:

```sh
git clone https://github.com/arch-err/calemdar.git
cd calemdar
just install
```

AUR and Homebrew packages are not yet available. Release binaries are
published via goreleaser on tagged versions — see
[releases](https://github.com/arch-err/calemdar/releases).

## Quickstart

```sh
calemdar config init                                                    # write default config, drops into $EDITOR
calemdar setup                                                          # scaffold vault folders
cp examples/calemdar.service ~/.config/systemd/user/calemdar.service    # install the unit
systemctl --user enable --now calemdar.service                          # run the daemon
calemdar series new                                                     # first recurring series
```

Then add one Full Calendar **Local Calendar** source per subfolder under
`events/`, each with its own colour. Full walkthrough:
[docs/quickstart](https://arch-err.github.io/calemdar/quickstart/).

## How it works

```
 recurring/workout.md ──▶  reconcile  ──▶  events/health/2026-05-03-workout.md
   (recurrence rule)                        events/health/2026-05-05-workout.md
                                            events/health/2026-05-07-workout.md
                                             │
                                             └─▶ Full Calendar reads these
```

Recurring roots live in `recurring/<slug>.md` (not a Full Calendar source).
The daemon expands each root into flat single-event files under
`events/<calendar>/`. When you drag an occurrence in Obsidian, it flips
`user-owned: true` and the server leaves it alone forever. Past is
immutable.

Full architecture doc: [docs/architecture](https://arch-err.github.io/calemdar/architecture/).

## Config

Minimum viable config at `~/.config/calemdar/config.yaml`:

```yaml
vault: /home/you/obsidian/my-vault
```

Everything else defaults sanely. Every key:
[docs/configuration](https://arch-err.github.io/calemdar/configuration/).

## Docs

Full docs rendered at **[arch-err.github.io/calemdar](https://arch-err.github.io/calemdar/)**.

- [Quickstart](https://arch-err.github.io/calemdar/quickstart/)
- [Configuration](https://arch-err.github.io/calemdar/configuration/)
- [CLI reference](https://arch-err.github.io/calemdar/cli/)
- [Architecture](https://arch-err.github.io/calemdar/architecture/)
- [Schema](https://arch-err.github.io/calemdar/schema/)
- [Design](https://arch-err.github.io/calemdar/design/)
- [FAQ](https://arch-err.github.io/calemdar/faq/)

## License

MIT. See [LICENSE](LICENSE).

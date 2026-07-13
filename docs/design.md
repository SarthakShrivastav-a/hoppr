# hoppr — design

This is the reasoning behind the code. It records the decisions that are
expensive to reverse, and the ones a future contributor is most likely to
undo without realizing what they cost.

## The problem

Managing more than a couple of servers by hand converges on the same mess:

- Connection commands live in a notes app or a clipboard manager.
- Private keys live in `~/Downloads`, because that is where the cloud console
  put them, and nothing ever moved them.
- At least one command begins with `cd`, because the key path in it is relative
  to wherever the user happened to be standing.
- On Windows, at least one of those keys is readable by every account on the
  machine, and the user has no idea, because `chmod 600` appeared to work.

OpenSSH already solves the first three with `~/.ssh/config`. Almost nobody
maintains one, because editing it by hand is tedious and every tool that offers
to do it for them eventually damages it.

## The central decision

**`ssh_config` is the only source of truth, and hoppr never rewrites more of it
than it was asked to.**

Everything else follows from this.

Survey the category and one failure mode dominates every issue tracker: tools
parse the config into a struct with a field per keyword they know about, then
re-serialize the whole file from that struct. Every complaint falls out of that
single decision:

| Reported bug | Cause |
|---|---|
| `Include` directive silently deleted | not a field in the struct |
| `UseKeychain`, `SetEnv`, `UserKnownHostsFile` dropped | not fields in the struct |
| All keywords lowercased | struct field names round-tripped as keys |
| Blank lines injected between every directive | re-serialization with a naive writer |
| Symlinked config dereferenced and replaced | write path did not preserve the link |
| Renaming a host corrupted an unrelated `ProxyJump` | blind string substitution over the file |
| Absolute Windows paths written into `IdentityFile` | no tilde normalization; breaks on sync to Linux/WSL |

The alternative some tools reach for — "we do not touch your config, we keep our
own database" — trades a corruption bug for a synchronization bug. The database
drifts the moment the user edits their config by hand, and now the tool is
lying.

hoppr does neither. It parses into a syntax tree in which **every node retains
the raw text it was parsed from**, including indentation, the key/value
separator, same-line comments, and keywords it has never heard of. Rendering an
untouched node replays its bytes verbatim. Only edited nodes are regenerated,
and even they preserve the original indentation and separator.

The property is pinned by `TestRoundTripIsLossless`, which parses a deliberately
hostile config — tabs and spaces, `=` separators, a quoted path containing both
a space and a `#`, an unknown keyword, a `Match` block, no trailing newline — and
asserts the output is byte-identical. `TestEditingOneHostLeavesEverythingElseByteIdentical`
is the other half.

**If you are changing `internal/sshconfig`, this is the invariant. Do not trade
it away for a cleaner-looking writer.**

## Where metadata lives

Tags, notes, and colors have no home in `ssh_config` — OpenSSH rejects unknown
keywords inside a `Host` block. The obvious workaround is a sidecar file keyed
by alias, and it is wrong: rename a host by hand and the sidecar still points at
the old alias, so the tags silently vanish.

hoppr stores them as a comment *inside* the stanza:

```
Host prod
    # hoppr: tags=aws,prod; note=main API box
    HostName 203.0.113.10
```

The annotation moves, copies, and deletes with the stanza it describes, because
it is physically inside it. To OpenSSH, and to every other tool, it is a comment.
There is no second file and nothing to synchronize.

Unknown fields in the annotation are ignored rather than rejected, so a config
written by a newer hoppr does not break an older one.

## Why we shell out to `ssh`

hoppr never speaks the SSH protocol. Connecting builds an argument list and
executes the system `ssh`.

This is a constraint, not a shortcut. Reimplementing the client means
reimplementing the agent protocol, `ProxyJump` chains, `Match` block evaluation,
hardware-key support, `known_hosts` semantics, certificate auth, and every
`-o` option a user might already depend on — badly, and forever. Shelling out
means all of it works on day one, exactly as it already does.

It also means hoppr cannot break your ability to connect. The worst it can do is
build a wrong command line, and `--dry-run` will show you it.

The corollary: **`ssh`'s stderr goes straight to the terminal, unfiltered.** When
a connection fails, ssh's own message is the single most useful thing on screen.
Capturing it, or flashing it for a tenth of a second before redrawing a TUI over
it, is a documented complaint against nearly every tool in this space.

## Keys

This is the part nothing else in the category does, and the reason is probably
that it is unglamorous rather than that it is hard.

- **Detection is by content, not extension.** A key from `ssh-keygen` has no
  extension; one from AWS is a `.pem`; and plenty of `.pem` files are
  certificates. The PEM header is the only reliable signal.
- **The reverse index is the feature.** "I have four `.pem` files — which of them
  still matters, and what breaks if I delete this one?" is unanswerable today
  without grepping `ssh_config` by hand, so nobody deletes any of them and the
  pile grows. `keys.Index` answers it; `keys.Missing` answers the inverse and
  catches a config still pointing at a key left on the old laptop, before it
  becomes a baffling `Permission denied (publickey)`.
- **Adopting a key rewrites the config that referenced it.** Moving the key is
  the easy half. Without the rewrite, adopting a key silently breaks every host
  that used it — strictly worse than leaving it in `Downloads`.
- **Adopting sets `IdentitiesOnly yes`.** A host with an explicit `IdentityFile`
  but no `IdentitiesOnly` still offers every key the agent holds, in agent order,
  before the one the config names. A server permitting six attempts hangs up
  first. The resulting `Too many authentication failures` is baffling precisely
  because the config is visibly correct.

## Windows

Windows is a first-class target, and it is where most of the sharp edges are.

- **`chmod` is a no-op on NTFS.** Only the ACL matters. This is why "just chmod
  the key" is advice that appears to work and changes nothing, and why users get
  stuck on `UNPROTECTED PRIVATE KEY FILE` having done nothing wrong: a new file
  in `~/.ssh` *inherits* that directory's ACL, and a stock Windows install grants
  `Users` read access through it. `keys.Fix` strips inheritance and grants the
  file to one account, via `icacls` — the documented remedy, which we run for
  them instead of printing.
- **Paths are written as `~/...` with forward slashes**, never as
  `C:\Users\you\...`. An absolute Windows path in `IdentityFile` breaks the
  config the moment it is synced to Linux or read from WSL.
- **CRLF and BOM are detected.** OpenSSH can reject a config for either, and the
  error names the keyword rather than the invisible character that caused it.
- **The console is prepared before drawing.** A non-UTF-8 codepage makes Bubble
  Tea's input decoder produce `utf8.RuneError` and exit before the first frame;
  `conhost` has ANSI processing off by default and renders the UI as literal
  escape sequences. Both are one call each, and both are restored on exit.

## Bubble Tea v1, deliberately

v2 does not deliver `WindowSizeMsg` on Windows: its input path does not emit a
buffer-size event, and Windows has no `SIGWINCH`. The UI therefore cannot respond
to a resize at all. Until that is fixed, v1 is the correct choice and the pin is
intentional.

## Non-goals

- **No cloud, no account, no sync.** Nothing leaves the machine. This is also a
  positioning decision: the paid tools in this space charge a subscription for
  cloud sync, and some of them upload host inventories users did not expect them
  to.
- **No password authentication.** Not yet, and not without an OS keychain
  (Credential Manager / Keychain / Secret Service) behind it. Storing passwords
  in plaintext is the request most often made and most correctly refused. Doing
  it properly is a real feature; doing it lazily is a liability.
- **hoppr is not an SSH client, and will not become one.**

## Layering

```text
sshconfig  ← lossless AST, atomic writes. The only package where a bug can hurt you.
   ↑
host       ← connectable-host model, fuzzy search
   ↑
keys / probe / doctor / connect / forward
   ↑
tui / cli
```

Dependencies point one way. `sshconfig` knows nothing about hosts; `host` knows
nothing about the UI. `keys` depends on `host` only to answer "what uses this
key", and `connect` depends on `keys` only to resolve a pasted key path to where
the key actually is.

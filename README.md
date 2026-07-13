<div align="center">

# hoppr

**An SSH session manager for the terminal.**

Stop copy-pasting `ssh -i ~/Downloads/prod.pem ec2-user@203.0.113.10`.

[![ci](https://github.com/SarthakShrivastav-a/hoppr/actions/workflows/ci.yml/badge.svg)](https://github.com/SarthakShrivastav-a/hoppr/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/SarthakShrivastav-a/hoppr?include_prereleases&sort=semver)](https://github.com/SarthakShrivastav-a/hoppr/releases)
[![go reference](https://pkg.go.dev/badge/github.com/SarthakShrivastav-a/hoppr.svg)](https://pkg.go.dev/github.com/SarthakShrivastav-a/hoppr)
[![go report](https://goreportcard.com/badge/github.com/SarthakShrivastav-a/hoppr)](https://goreportcard.com/report/github.com/SarthakShrivastav-a/hoppr)
[![license](https://img.shields.io/github/license/SarthakShrivastav-a/hoppr)](LICENSE)

</div>

---

## Why

If you manage more than two servers, you know the shape of this problem. The connection commands live in a notes app or a clipboard manager. The keys are in `~/Downloads`, because that is where the cloud console put them. And at least one command starts with a `cd`, because the key path is relative to wherever you happened to be standing:

```text
cd .\Downloads\ ssh -i dev.pem ec2-user@dev-api.example.com
ssh -i prod.pem ec2-user@203.0.113.10
```

OpenSSH already solved this. It is called `~/.ssh/config`, and almost nobody maintains one, because editing it by hand is tedious and every tool that offers to do it for you eventually eats a comment, drops an `Include`, or lowercases half your keywords.

`hoppr` is a terminal UI over the config you already have. It edits the real file, surgically. It finds the keys you have scattered around your disk and tells you which hosts still use them. And it tells you *why* your connection is about to fail, before it does.

## See it

```text
$ hoppr import "cd .\Downloads\ ssh -i dev.pem ec2-user@dev-api.example.com"
added dev-api → dev-api.example.com
connect with: ssh dev-api

$ hoppr keys ls
KEY          TYPE     PERMS     USED BY   PATH
dev.pem *    ed25519  ok        dev-api   C:\Users\me\Downloads\dev.pem
prod.pem     rsa      TOO OPEN  prod      C:\Users\me\.ssh\keys\prod.pem
ancient.pem  rsa      ok        —         C:\Users\me\.ssh\keys\ancient.pem

* lives outside ~/.ssh — `hoppr keys adopt` moves it and repoints the hosts that use it

$ hoppr doctor
FAIL  key permissions too open: prod.pem
      readable by BUILTIN\Users — ssh will refuse this key
      fixable: lock prod.pem to your account

warn  1 host names a key but does not set IdentitiesOnly
      prod. ssh will still offer every key in your agent first; if the agent
      holds more keys than the server allows attempts, the connection dies with
      "Too many authentication failures" before the right key is tried.

$ hoppr doctor --fix
fixed: key permissions too open: prod.pem
fixed: 1 host names a key but does not set IdentitiesOnly

$ hoppr            # the interactive list
```

## Install

```bash
# Latest release
go install github.com/SarthakShrivastav-a/hoppr/cmd/hoppr@latest

# From source
git clone https://github.com/SarthakShrivastav-a/hoppr.git
cd hoppr && go build -o hoppr ./cmd/hoppr
```

Requires Go 1.25+ to build. Pre-built binaries for Linux, macOS, and Windows on amd64 and arm64 are attached to every [release](https://github.com/SarthakShrivastav-a/hoppr/releases) — download, unzip, drop on `PATH`.

You also need an `ssh` binary, which you already have. On Windows, that is the OpenSSH Client optional feature (`Settings → Apps → Optional features`), and it is installed by default on Windows 10 and 11.

## The one idea

**Your `~/.ssh/config` is the only source of truth, and hoppr never rewrites more of it than you asked it to.**

Every other tool in this category parses your config into a struct and re-serializes the whole file. That one decision is where all of their bugs come from — dropped `Include` directives, dropped `UseKeychain`, lowercased keywords, injected blank lines, dereferenced symlinks, corrupted `ProxyJump` references.

hoppr parses into a syntax tree that keeps the raw text of every line, including the lines it does not understand. Changing one `HostName` rewrites one line. Everything else in the file — your comments, your indentation, your `=` signs, your keywords from a version of OpenSSH newer than this binary — comes back byte for byte. There is a test that asserts exactly that, against a config designed to be hostile.

Not sure you trust it yet? `hoppr --read-only` will do everything except write.

## What it does

### Manage hosts

Fuzzy search across aliases, hostnames, tags, and notes — type `prd` and find `prod-api`. Tag hosts into groups. Live reachability indicators. Vim keys and arrow keys, everywhere.

Tags and notes are stored as a comment inside the stanza they describe:

```
Host prod
    # hoppr: tags=aws,prod; note=main API box
    HostName 203.0.113.10
```

So they travel with the host when you move it, copy it, or delete it — and to every other tool, including OpenSSH, they are just a comment. There is no second database to fall out of sync with your config.

### Manage keys

This is the part nothing else in this space does.

`hoppr keys ls` scans `~/.ssh`, `Downloads`, `Desktop`, and `Documents` for private keys — detected by content, not by extension, because the key from `ssh-keygen` has no extension and half the `.pem` files on your disk are certificates. For each one it shows the fingerprint, the type, **which hosts use it**, and whether its permissions are actually safe.

`hoppr keys adopt` moves a stray key into `~/.ssh/keys`, locks it down, and **repoints every host that referenced it**. That last part is what makes it safe to do — moving a key without it silently breaks the hosts that used it.

### Tell you what is about to break

`hoppr doctor` checks for the failures that produce genuinely useless error messages:

| You will see | What is actually wrong |
|---|---|
| `Permission denied (publickey)` | A host names an `IdentityFile` that is not there |
| `Too many authentication failures` | A host names a key but not `IdentitiesOnly`, so ssh offers every agent key first and the server hangs up before reaching the right one |
| `Bad configuration option` | A CRLF or a byte-order mark that a Windows editor added |
| `UNPROTECTED PRIVATE KEY FILE` | A key readable by more than just you |
| *Your edit silently does nothing* | The same alias is declared twice, and OpenSSH applies the first one |

`hoppr doctor --fix` repairs the mechanical ones. It exits non-zero when it finds something, so it works as a pre-flight check in a script.

## Windows

Windows is a first-class target, not a port. It is also where most of the sharp edges are, and where the other tools in this category either do not run or do not help.

- **`chmod 600` does nothing on Windows.** Only the NTFS ACL matters, which is why "just chmod the key" is advice that appears to work and changes nothing. hoppr audits the real ACL and fixes it with `icacls`, stripping the inherited permissions that a new key picks up from an open `~/.ssh` — which is how people hit `UNPROTECTED PRIVATE KEY FILE` without having done anything wrong.
- **Config paths are written with `~/` and forward slashes**, never as `C:\Users\you\...`, so the same config works when you sync it to a Linux box or read it from WSL.
- **CRLF and BOM are detected**, because OpenSSH can reject a config for either and the error it gives names the keyword, not the invisible character.
- **The console is put into UTF-8 mode** before drawing, and ANSI processing is enabled for `conhost`. Without that, a non-English codepage crashes the input decoder and `conhost` renders the UI as literal escape sequences.

CI runs the full test suite on Windows, macOS, and Linux.

## Commands

```text
hoppr                                open the interactive list
hoppr ls [query] [--tag t] [--json]   list hosts
hoppr add <alias> --host H [-u -p -i -J -t]
hoppr import "<ssh command>"          turn a command you already use into a host
hoppr rm <alias>
hoppr connect <alias> [--dry-run]

hoppr keys ls [--json]                every key, its fingerprint, and what uses it
hoppr keys fix [key|--all]            lock key permissions down
hoppr keys adopt [key|--all]          move a stray key in and repoint its hosts

hoppr doctor [--fix]                  diagnose what is about to break

  -c, --config    operate on a different ssh_config
      --read-only never modify anything
```

Every list command takes `--json`, because people drive SSH config from Ansible and from shell scripts, and a manager that only works when a human is watching cannot be part of that.

## What it is not

- **Not a second copy of your hosts.** There is no hoppr database. Delete hoppr and your config still works, exactly as it did.
- **Not an SSH client.** Connecting shells out to your system `ssh`, so your agent, your jump hosts, your hardware keys, your `Match` blocks, and your `known_hosts` all behave exactly as they already do. A manager that reimplements the client gets to reimplement all of that, badly, forever.
- **Not a cloud service.** Nothing leaves your machine. There is no account, no sync, and no subscription.

## Architecture

```text
sshconfig  →  host    →  tui / cli
lossless      model       bubbletea
AST + IO      + fuzzy     + cobra
              search
                ↑
    keys    ────┤   scan, fingerprint, ACL audit, adopt
    probe   ────┤   TCP reachability
    doctor  ────┘   diagnosis + mechanical fixes
    connect ────┘   parse ad-hoc commands, exec ssh
```

`sshconfig` is the load-bearing package, and the only one where a bug can hurt you. It is also the most heavily tested.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The bar for anything touching `internal/sshconfig` is a test proving the round trip is still lossless.

## License

[MIT](LICENSE).

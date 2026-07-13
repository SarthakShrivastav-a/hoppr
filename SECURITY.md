# Security

## What hoppr touches

hoppr reads and writes `~/.ssh/config` and the files it includes, reads private
keys from disk, and changes file permissions. That is a sensitive blast radius,
so it is worth being precise about what it does and does not do.

**It does:**

- Parse private keys locally, to read their type and fingerprint. Encrypted keys
  are never decrypted — hoppr does not ask for, accept, or store a passphrase.
- Change permissions on key files, on request (`keys fix`, `keys adopt`,
  `doctor --fix`). On Unix this is `chmod 0600`; on Windows it is `icacls`,
  stripping inherited ACL entries and granting the file to your account.
- Move key files into `~/.ssh/keys`, on request (`keys adopt`), and rewrite the
  `IdentityFile` lines that referenced them.
- Execute the system `ssh` binary to connect.

**It does not:**

- Send anything anywhere. There is no network call other than a TCP reachability
  probe to the hosts already in your config, and the `ssh` connections you ask
  for. No telemetry, no analytics, no account, no sync.
- Store passwords or passphrases. Anywhere. In any form.
- Read the contents of a key beyond what is needed to identify it.
- Modify anything at all under `--read-only`.

## Reporting a vulnerability

Please report privately via
[GitHub Security Advisories](https://github.com/SarthakShrivastav-a/hoppr/security/advisories/new)
rather than opening a public issue.

Things worth reporting, beyond the obvious:

- Any path where hoppr writes a key with permissions wider than owner-only, even
  transiently. A key that is briefly world-readable is a key that leaked.
- Any path where a config write is not atomic, or could leave a truncated
  `ssh_config` behind. That is a lockout, not a cosmetic bug.
- Any way to get hoppr to execute something other than the intended `ssh` binary,
  or to inject arguments into it via a crafted config or a pasted command.
- Any way to make the config parser produce a file that OpenSSH interprets
  differently than hoppr displayed it. Showing the user one thing and writing
  another is the worst outcome this tool has.

## Supported versions

The latest release. hoppr is pre-1.0 and moves forward rather than backporting.

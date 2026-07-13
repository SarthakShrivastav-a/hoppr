# Changelog

Notable changes. Releases are cut automatically from conventional commits on
`main`, so this file records the shape of each version rather than every commit.

## Unreleased

### Added

- **Lossless `ssh_config` editing.** Parse into a syntax tree that keeps the raw
  text of every line, including unknown keywords. Changing one `HostName`
  rewrites one line; the rest of the file comes back byte for byte.
- **`Include` support.** Hosts split across `~/.ssh/conf.d/*.conf` are found, and
  edits are saved back to the file that declares them.
- **Tags and notes** stored as an in-band comment inside the stanza, so there is
  no sidecar database to drift out of sync with the config.
- **`hoppr import`.** Turn an ad-hoc `ssh -i key.pem user@host` command — leading
  `cd` and all — into a proper Host entry. A bare or relative key name is
  resolved to where the key actually is.
- **Key inventory.** Find private keys across `~/.ssh`, `Downloads`, `Desktop`,
  and `Documents`, detected by content rather than extension. Shows fingerprint,
  type, which hosts use each key, and which keys nothing uses.
- **Key adoption.** Move a stray key into `~/.ssh/keys`, lock its permissions,
  and repoint every host that referenced it.
- **Permission repair that works on Windows.** Audit the real NTFS ACL and fix it
  with `icacls`, rather than calling `chmod` and pretending.
- **`hoppr doctor`.** Diagnose the failures with useless error messages:
  `Permission denied (publickey)` from a missing key, `Too many authentication
  failures` from a missing `IdentitiesOnly`, `Bad configuration option` from a
  CRLF or BOM, `UNPROTECTED PRIVATE KEY FILE` from an inherited ACL, and a
  duplicate alias silently shadowing your edit. `--fix` repairs the mechanical
  ones.
- **Bubble Tea TUI** with fuzzy search over aliases, targets, tags, and notes;
  tag filtering; live reachability; vim and arrow keys; and a clean handoff to
  the real `ssh`.
- **`--read-only`**, which does everything except write.
- **JSON output** on every list command, so hoppr is usable from a script.

### Notes

- Pinned to Bubble Tea v1: in v2, Windows never receives a resize event, so the
  UI cannot respond to a terminal resize at all.
- Connecting always shells out to the system `ssh`. Agent, jump hosts, hardware
  keys, `Match` blocks, and `known_hosts` therefore behave exactly as they
  already do.

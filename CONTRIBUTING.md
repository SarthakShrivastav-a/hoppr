# Contributing

Thanks for looking. Bug reports, fixes, and features are all welcome.

## Getting set up

```bash
git clone https://github.com/SarthakShrivastav-a/hoppr.git
cd hoppr
make build      # -> bin/hoppr
make test
make lint       # gofmt + go vet + go mod tidy, which is what CI runs
```

Go 1.25+.

To try changes without touching your real config, point hoppr somewhere else:

```bash
HOPPR_CONFIG=/tmp/test-config ./bin/hoppr
# or
./bin/hoppr -c /tmp/test-config
```

## The one rule

**A parse/render round trip through `internal/sshconfig` must be byte-for-byte lossless.**

This is not a style preference. A user's `~/.ssh/config` is hand-maintained territory: it holds comments, deliberate blank lines, blocks written by other tools, and keywords from a version of OpenSSH newer than this binary. If hoppr reformats any of that, it has damaged something the user cares about and did not ask us to touch.

Every bug people file against tools in this category — dropped `Include` directives, dropped `UseKeychain`, lowercased keywords, injected blank lines, dereferenced symlinks, corrupted `ProxyJump` references — comes from re-serializing the whole file from a lossy struct. hoppr does not do that, and any change that would let it start is not going in.

Concretely, if you touch `internal/sshconfig`:

- `TestRoundTripIsLossless` must still pass. It parses a deliberately hostile config — tabs and spaces, `=` separators, same-line comments, a quoted path holding both a space and a `#`, an unknown keyword, a `Match` block, no trailing newline — and asserts the output is identical to the input.
- If you add support for a new shape, add it to that fixture.
- `TestEditingOneHostLeavesEverythingElseByteIdentical` is the other half: changing one host must not perturb a single byte anywhere else.

## Commits

Conventional commits, because the release pipeline reads them to pick the next version:

```text
feat(keys): ...     -> minor bump
fix(tui): ...       -> patch bump
feat(cli)!: ...     -> major bump
docs: / test: / chore: / ci:
```

Say *why* in the body, not what — the diff already says what. A commit that explains the failure it prevents is worth five that restate their own code.

## Testing

- `go test ./...` before opening a PR; `make race` if you touched anything concurrent.
- New behaviour needs a test. New behaviour in `internal/sshconfig` needs a test that pins the round trip.
- Test names should say what breaks, not what is called. `TestHashInsideQuotesIsNotAComment` beats `TestParse4`.

## Windows

Windows is a supported platform, not a courtesy, and it is where most of the sharp edges live: ACLs instead of mode bits, CRLF, codepages, and a console that does not speak ANSI by default. CI runs the suite on Windows, macOS, and Linux, and a change that only works on one of them is not finished.

If you cannot test on Windows, say so in the PR and someone will.

## Scope

hoppr manages the config and the keys. It is not an SSH client, and it will not become one — connecting shells out to the system `ssh`, and that is what makes everything you already rely on keep working.

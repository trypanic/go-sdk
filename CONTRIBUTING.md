# Contributing to go-sdk

Thanks for helping. This file covers the workflow; the
[README](./README.md) covers setup details and
[AGENTS.md §8](./AGENTS.md#8-working-inside-this-repo-for-sdk-contributors--agents-extending-the-sdk)
the conventions for adding or changing a package.

**Security problems are not reported here.** Use GitHub private
vulnerability reporting as described in [`SECURITY.md`](./SECURITY.md).

## Set up

Tool versions are pinned in [`.prototools`](./.prototools) and
[`Taskfile.yml`](./Taskfile.yml). Follow
[README "Developer setup"](./README.md#developer-setup), then:

```bash
task tools:check          # active Go is the pinned, supported toolchain
task precommit:install    # pre-commit and commit-msg hooks
```

## Before you open a pull request

```bash
task test -- <pkg>        # unit tests of the package you changed (go test -short)
task check                # the same commands as CI: unit + race tests, vet,
                          # staticcheck + semgrep, govulncheck, tidy, hooks
task release:check        # if you touched an exported API (gorelease)
```

- Unit tests live next to the code (`foo.go` ↔ `foo_test.go`) and must pass
  with `-short` and no external services. Tests that need Postgres, MongoDB
  or RabbitMQ are skipped under `-short` or use the `integration` build tag.
- A bug fix comes with a regression test that fails without the fix.
- Do not weaken or delete a test to make a change pass.
- `go.mod`/`go.sum` change only through `task deps:update` (all packages
  share one module; never run several `go get` in parallel).
- Static-analysis findings, rule severities and the runner's knobs are
  documented in
  [`SECURITY.md`](./SECURITY.md#static-analysis-contributors). Never commit
  with `--no-verify`; fix the rule in `.semgrep.yml` instead if it is wrong.

## Commit messages

[Conventional Commits](https://www.conventionalcommits.org), enforced by the
`commit-msg` hook: `feat:`, `fix:`, `fix(deps):`, `perf:`, `refactor:`,
`docs:`, `test:`, `build:`, `ci:`, `chore:`, optionally scoped by package
(`fix(httprequest): …`). Mark a breaking change with `!` and a
`BREAKING CHANGE:` footer.

release-please derives the next version and the release notes in
`CHANGELOG.md` from these messages (README "Releases"). Never edit
`CHANGELOG.md` or `.release-please-manifest.json` by hand.

## Pull requests

- One logical change per pull request, with its tests and documentation (the
  package `README.md` is the contract users read).
- The [`ci`](./.github/workflows/ci.yml) workflow must be green: build, vet,
  race tests on Go 1.26.x and 1.27.x, static analysis, govulncheck, and
  `go mod tidy -diff` / `go mod verify`. The `gorelease` job reports API
  changes and does not block.

## Local infrastructure integration tests

Run `task test:integration:pull` once to download the digest-pinned PostgreSQL
18.6, MongoDB 9.0.4, RabbitMQ 4.4.0 and Testcontainers Ryuk 0.14.0 images.
Then run `task test:integration`. Tests require a local Docker Unix socket,
bind ephemeral ports explicitly to `127.0.0.1`, use disposable state, and fail
if images are not cached. Runtime Docker inspection checks the service and
Ryuk bindings; the client connection hostname alone is insufficient.
They never use a configured application database or broker. Testcontainers
v0.44.0 is an integration-only dependency in the single SDK module.

The `integration` build tag excludes the suites from normal builds; `-short`
also skips them before Docker access. To verify this guard, run
`go test -tags integration -short ./...`. Containers have bounded readiness
and cleanup contexts; clients close before their containers. Keep Ryuk enabled
for crash cleanup. The integration helper uses the pinned v0.44.0 exported
`ReaperProvider` hook because Ryuk has no HostIP configuration. Its single
localized deprecation exception preserves upstream session reuse, socket
mounts, auto-remove and cleanup; replace it when an upstream binding option
becomes available. See the [upstream reaper implementation](https://github.com/testcontainers/testcontainers-go/blob/v0.44.0/reaper.go).
To repeat the real suites with shuffled ordering, run
`go test -race -tags integration -run Integration -count=2 -shuffle=on -timeout=10m ./postgres ./mongo ./messaging`.

For a major module-path migration, `task release:check` resolves both the module
path and version from the previous tag, preserving the cross-major API report.
Use `task release:check RELEASE_VERSION=v2.0.0` to validate the proposed major
against that baseline. Separately, `task release:check BASE=none RELEASE_VERSION=v2.0.0`
validates the initial v2 release without an API comparison; it does not replace
the historical comparison. Every release check requires a clean working tree.

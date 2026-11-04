# R-DEPS review loop 2 evidence

Base reviewed head: `ac8ab8e`. Fixes: `40580e0`, `f32d1f3`.
`checks.jsonl` records commands, actual exit codes and duration. Logs retain
stdout/stderr, with trailing whitespace and terminal blank lines normalized by
the required repository hooks. Mixed indentation in the two negative-control
logs is expanded to spaces for `git diff --check`; original files remain in
`/tmp/DEPS-loop2`.
Expected negative controls are not passing production checks.

- `integration-repeat.log`: real Docker publications, race detection and two
  shuffled runs of all three service suites plus the lifecycle modifier test.
- `wildcard-regression.log` / `ryuk-wildcard-regression.log`: runtime failures
  on `0.0.0.0` for the service and reaper independently (exit 1). Decode the
  matching `.patch.b64` and apply in an isolated export of `f32d1f3` to repeat
  the logged command. Base64 prevents scanners from treating patches as Go.
- `crash-probe.go.txt`: temporary test source, deliberately excluded from Go
  package discovery. It was compiled with `integration,auditcrash`, run as a
  standalone test binary and exited 23 without cleanup callbacks. The session
  ID and initial loopback bindings are in `crash-probe.log`. The first cleanup
  inspection was empty; `crash-cleanup-assertion.json` records an additional
  explicit assertion that both the service and Ryuk are absent.
- `ci-matrix.py` reads the committed workflow instead of duplicating its job
  commands. Its local toolchain locations are environment-specific. The log
  proves the old auto-selection fails the new version assertion, then runs
  both matrix jobs and all tagged short tests with each fixed binary.
- `module-graph-unchanged.txt`: all selected versions remain identical to the
  original R-DEPS graph. Moby api/client become direct requirements only.
- Release checks ran on the clean implementation tree at `f32d1f3`.

No application credentials, external application services, daemon changes,
firewall changes, dependency upgrades or release actions were used.

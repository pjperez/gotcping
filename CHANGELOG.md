# Changelog

All notable changes to this project are documented here. The format is loosely
based on [Keep a Changelog](https://keepachangelog.com/), and this project
adheres to [Semantic Versioning](https://semver.org/).

## [0.7.0] - 2026-08-09

### Added — probing engine
- **Concurrent probing** (`-P N`): probes run through a worker pool with up to
  N concurrent dials while preserving the `-i` pacing between launches. Long
  runs finish N times faster.
- **Sub-second timeouts**: `-timeout` now accepts fractions of a second
  (>= 0.001), enabling millisecond-level connect RTT measurement.
- **TLS handshake timing** (`-tls`): after the TCP connect, gotcping performs a
  real TLS handshake and reports the handshake duration per probe. `-sni`
  overrides the SNI/ServerName (defaults to the target host) and `-insecure`
  skips certificate verification for self-signed endpoints.
- **Kernel RTT** (`-rtt`): reads `TCP_INFO` via `getsockopt` on Linux
  (`golang.org/x/sys/unix`) and reports the average kernel-measured RTT — a
  lower-bound estimate of path RTT that excludes userspace and queueing delay.
  Silently disabled on other platforms.
- **Source binding** (`-source` / `-source-port`): bind the local endpoint by
  IP or interface name; `-source-port` accepts a fixed port or a `start-end`
  range (a random port per dial avoids collisions under concurrency).
- **Exit-loss threshold** (`-exit-loss percent`): exits with code `4` when the
  % of failed probes exceeds the threshold — for scripts, cron, and health
  checks.

### Added — statistics & reporting
- **p95 / p99 percentiles** added to every summary (JSON, JSONL, CSV, text).
- **ASCII RTT histogram** (`-hist`): 20 equal-width bins rendered after the
  summary.
- **JSONL mode** (`-jsonl`): one JSON object per line — a `probe` event per
  probe (with timestamp and TLS/error fields) and a `summary` event.
- **CSV mode** (`-csv`): one row per probe plus a summary row; the separator is
  auto-detected as comma or semicolon for locale-friendly spreadsheets.
- **Timestamps** (`-t`): per-probe lines include a UTC timestamp.
- **Multi-target file mode** (`-file targets.txt`): probes `host[:port]` per
  line (`#` comments and bracketed IPv6 supported), one summary block per
  target, and an exit code that is the most severe across all targets.

### Added — governance & CI
- New dependency `golang.org/x/sys v0.47.0` (for `TCP_INFO` on Linux).

### Changed
- Statistics now also report p95/p99 and, when `-rtt` is used, the average
  kernel RTT.
- README rewritten: full flags table, p95/p99 + kernel RTT stats, exit code `4`,
  concurrency/TLS/source-binding/multi-target notes, and examples for every new
  mode.

### Tests
- New unit tests: `parseSource`, `parsePortRange`, `parseTargetLine`,
  `readTargets`, `computeHistogram`, `kernelRTT` on a non-Linux stub, and
  `emitSummary` exit-code logic (0/3/4).
- New integration tests: `PingLocalTCP` (real local listener, concurrency +
  sub-second timeout + source binding) and `PingTLS` (httptest TLS server,
  handshake timing). 24 tests total, all passing with `-race`.

### Security / robustness (carried over from 0.5.x; now documented)
- Resolve-once dial (no DNS-rebinding TOCTOU between validation and connect).
- Bounded sample retention (max 100,000 samples) to cap memory in infinite mode.
- Input validation for port, timeout, deadline, and interval.
- Terminal-control sanitization of the hostname in output lines.
- Signal (SIGINT/SIGTERM) and deadline handling that still prints the summary.

## [0.6.0] - 2026-07-03

### Added — capabilities
- `-json` output mode: emits a single machine-readable JSON object on stdout
  (host, port, probes_sent, successful, failed, percent_failed, min/avg/
  median/max/stddev/jitter, and p25/p50/p75/p90 in ms). Per-probe lines and
  failures are suppressed/redirected so stdout stays pure JSON.
- `-q` quiet mode: suppresses per-probe lines, prints only the summary.
- `-i` configurable inter-probe interval in seconds (>= 0.001), default `1`.
- `-4` / `-6` flags to force IPv4 or IPv6 when resolving the host (mutually
  exclusive). Without either flag, behavior is unchanged.
- `-version` flag. Version is injected at build time via ldflags
  (`-X main.Version=...`) rather than hardcoded, so release binaries report the
  tag version and source builds report `dev`.
- **Standard deviation** and **jitter** (mean absolute delta of consecutive
  RTTs) are now part of every summary.

### Added — governance & CI
- `LICENSE` (MIT) — the project was previously unlicensed.
- `SECURITY.md` vulnerability reporting policy.
- `.golangci.yml` (errcheck, govet, ineffassign, staticcheck, unused,
  misspell, revive).
- CI runs on a **Windows + Linux + macOS** matrix and includes `go test -race`
  and a `golangci-lint` job, in addition to `govulncheck`.
- Automated **release pipeline** (`.github/workflows/release.yml`): pushing a
  version tag cross-compiles `linux/amd64`, `linux/arm64`, `windows/amd64`,
  `darwin/amd64`, and `darwin/arm64` (stripped, `-trimpath`, version injected)
  and publishes a GitHub Release with this changelog as the body.
- `CHANGELOG.md`.

### Changed
- **stdout/stderr separation**: per-probe success lines and the summary go to
  stdout; diagnostics (per-probe failures, resolve errors, "all probes failed")
  go to stderr. Makes the tool scriptable and pipe-friendly.
- Failure lines now report the real underlying error (e.g.
  `dial tcp ...: connect: connection refused`) instead of always saying
  "Received timeout while connecting...".
- Statistics refactored into a pure, tested `computeStats` function (median is
  reused as the 50th percentile); samples are stored in nanoseconds and
  rendered in milliseconds.
- README rewritten with full flags/stats/exit-code tables and refreshed output
  from 0.6.0 runs (including `-json` and `-q -i` examples).
- `go.mod` loosened from `go 1.25.10` to `go 1.25`.

### Tests
- Added coverage for `resolveFamily`, `validateInterval`, and `computeStats`
  (min/avg/max, median==p50, jitter for multi/single sample, failure
  accounting, empty-input error, JSON round-trip).

### Security / robustness (carried over from 0.5.x; now documented)
- Resolve-once dial (no DNS-rebinding TOCTOU between validation and connect).
- Bounded sample retention (max 100,000 samples) to cap memory in infinite mode.
- Input validation for port, timeout, deadline, and interval.
- Terminal-control sanitization of the hostname in output lines.
- Signal (SIGINT/SIGTERM) and deadline handling that still prints the summary.

## [0.5.1] - earlier

- Flexible argument parsing, README updates, first binaries published.

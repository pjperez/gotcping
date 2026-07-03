# Changelog

All notable changes to this project are documented here. The format is loosely
based on [Keep a Changelog](https://keepachangelog.com/), and this project
adheres to [Semantic Versioning](https://semver.org/).

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

### Security / robustness (carried over from 0.5.x; now documented)
- Resolve-once dial (no DNS-rebinding TOCTOU between validation and connect).
- Bounded sample retention (max 100,000 samples) to cap memory in infinite mode.
- Input validation for port, timeout, and deadline.
- Terminal-control sanitization of the hostname in output lines.
- Signal (SIGINT/SIGTERM) and deadline handling that still prints the summary.

## [0.5.1] - earlier

- Flexible argument parsing, README updates, first binaries published.

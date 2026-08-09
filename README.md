# TCPing

Yet another tcping tool - forked from https://github.com/hwsdien/gotcping

Measure your RTT / latency to any TCP endpoint. Connect RTT, TLS handshake time,
kernel TCP_INFO RTT, p95/p99, histograms, concurrent probing, and machine-readable
output — built for modern network diagnostics.

## Requirements

gotcping uses the awesome [stats](https://github.com/montanaflynn/stats)  library by [montanaflynn](https://github.com/montanaflynn) 

## Installing

### From source

    go install github.com/pjperez/gotcping@latest

### Running from source without installing

    go run github.com/pjperez/gotcping@latest <args>

## Binaries

### Windows and Linux

[gotcping 0.7.0](https://github.com/pjperez/gotcping/releases/tag/0.7.0)

## Usage

    gotcping -host host [-port port_number] [-count number_of_repetitions] [-timeout timeout_in_seconds] [-deadline max_seconds] [-i seconds] [-P concurrent_probes] [-tls [-sni name] [-insecure]] [-rtt] [-t] [-json | -jsonl | -csv] [-hist] [-source addr] [-source-port start[-end]] [-exit-loss percent] [-file targets.txt] [-4] [-6] [-version]

### Alternate (only host is mandatory and can be specified either as a flag or as a positional argument)

    gotcping host

### Flags

| Flag           | Default  | Description |
|----------------|----------|-------------|
| `-host`        | *(none)* | Host or IP address to test. Can also be given as a positional argument. |
| `-port`        | `80`     | TCP port to query (1–65535). |
| `-count`       | `10`     | Number of probes to send. `0` or negative means **infinite** (stop with Ctrl+C, or use `-deadline`). |
| `-timeout`     | `1`      | Per-probe dial timeout, in seconds (>= 0.001). Sub-second timeouts are supported. |
| `-deadline`    | `0`      | Stop after this many seconds regardless of `-count`. `0` means no deadline. |
| `-i`           | `1`      | Seconds between probes (>= 0.001). |
| `-P`           | `1`      | Concurrent probes: run up to N dials in parallel (worker pool) while keeping the `-i` pacing. |
| `-tls`         | `false`  | Also measure the TLS handshake: after connecting, perform a TLS handshake and report both the TCP connect RTT and the handshake time. |
| `-sni`         | host     | SNI / ServerName to use for the TLS handshake. Defaults to the target host. |
| `-insecure`    | `false`  | Skip TLS certificate verification (for self-signed / private endpoints). |
| `-rtt`         | `false`  | Read the kernel TCP_INFO RTT (Linux) and report the average over successful probes. |
| `-t`           | `false`  | Print a UTC timestamp on each per-probe line. |
| `-q`           | `false`  | Quiet: suppress per-probe lines, print only the summary. |
| `-json`        | `false`  | Emit results as a single JSON object on stdout (for automation). Suppresses per-probe lines. |
| `-jsonl`       | `false`  | Emit one JSON object per line: a `probe` event per probe and a `summary` event at the end. |
| `-csv`         | `false`  | Emit CSV: one row per probe plus a summary row (auto-detects comma/semicolon separators). |
| `-hist`        | `false`  | Print an ASCII RTT histogram (20 equal-width bins) after the summary. |
| `-source`      | *(none)* | Bind the local source address: an IP or a local interface name (e.g. `eth0`). |
| `-source-port` | *(none)* | Bind a local source port, or a range `start-end` (a random port in the range per dial). |
| `-exit-loss`   | *(none)* | If the % of failed probes exceeds this threshold, exit with code `4`. |
| `-file`        | *(none)* | Read multiple targets from a file (`host[:port]` per line, `#` for comments). One summary block per target. |
| `-4`           | `false`  | Force IPv4 when resolving the host. Mutually exclusive with `-6`. |
| `-6`           | `false`  | Force IPv6 when resolving the host. Mutually exclusive with `-4`. |
| `-version`     | `false`  | Print the version and exit. |

### Statistics reported

For every run, gotcping reports: probes sent, successful, failed, % failed, and the **min / average / median / max** RTT plus **standard deviation** and **jitter** (mean absolute difference between consecutive RTTs), and the **25th / 50th / 75th / 90th / 95th / 99th percentiles**. With `-rtt` (Linux) it additionally reports the **average kernel TCP_INFO RTT** — a lower-bound estimate of the network path RTT that excludes userspace and queueing delay. All latencies are in milliseconds.

### Output streams

Per-probe success lines and the summary go to **stdout**. Diagnostics (per-probe failures, resolve errors, and the "all probes failed" message) go to **stderr**. This keeps stdout a clean, parseable stream — useful for piping the summary or `-json` / `-jsonl` / `-csv` output into other tools.

### Exit codes

| Code | Meaning |
|------|---------|
| `0`  | Success. |
| `1`  | Usage error (missing/invalid arguments). |
| `2`  | Host could not be resolved. |
| `3`  | All probes failed (host not accepting connections on the given port). |
| `4`  | The % of failed probes exceeded the `-exit-loss` threshold. |

### Notes

- The host is resolved **once** and the resulting IP is used for every probe, so the statistics describe a single network path (no per-probe DNS variance and no DNS-rebinding TOCTOU between validation and dial).
- `-P N` runs up to N dials concurrently through a worker pool; the `-i` pacing between probe launches is preserved, so long runs finish N times faster.
- TLS probes (`-tls`) perform a real TLS handshake against the target and report the handshake duration per probe. Use `-sni` to override the SNI name and `-insecure` for self-signed certificates.
- Kernel RTT (`-rtt`) reads `TCP_INFO` via `getsockopt` and is only available on **Linux** (silently disabled elsewhere).
- `-source` accepts either a local IP or an interface name (resolved via `net.Interfaces()`). `-source-port` binds a fixed port or a `start-end` range; a random port is chosen per dial to avoid collisions under concurrency.
- `-exit-loss 5` makes the process exit `4` when more than 5% of probes fail — useful as a health-check alert in scripts and cron.
- `-file targets.txt` probes many endpoints in one run. Each line is `host[:port]` (`#` comments and bracketed IPv6 like `[::1]:8443` are supported); lines without a port use the `-port` default. The final exit code is the most severe across all targets.
- `-4` / `-6` select which address family to dial; if the host resolves only to the other family, the run fails with exit code `2` (e.g. `error: no ip6 address found for <host>`).
- In infinite mode (`-count 0`), statistics are computed over the most recent **100,000** successful samples to keep memory bounded. For finite runs, all samples are used.
- Hitting Ctrl+C (or SIGTERM), or reaching `-deadline`, stops the run cleanly and still prints the full summary.

### Examples

#### Specify all parameters

    D:\gotcping> .\gotcping.exe -host github.com -count 5 -port 443
    Probe 1: Connected to github.com:443, RTT=6.12ms
    Probe 2: Connected to github.com:443, RTT=5.57ms
    Probe 3: Connected to github.com:443, RTT=6.42ms
    Probe 4: Connected to github.com:443, RTT=5.90ms
    Probe 5: Connected to github.com:443, RTT=5.26ms

    Probes sent: 5
    Successful responses: 5
    % of requests failed: 0
    Min response time: 5.2623ms
    Average response time: 5.85478ms
    Median response time: 5.8963ms
    Max response time: 6.4232ms
    Std deviation: 407.259µs
    Jitter (mean abs delta): 642.149µs

    90% of requests were faster than: 6.30308ms
    75% of requests were faster than: 6.1229ms
    50% of requests were faster than: 5.8963ms
    25% of requests were faster than: 5.5692ms

#### Specify only host (positional argument)

    D:\gotcping> .\gotcping.exe github.com
    Probe 1: Connected to github.com:80, RTT=5.36ms
    Probe 2: Connected to github.com:80, RTT=6.42ms
    Probe 3: Connected to github.com:80, RTT=5.38ms
    Probe 4: Connected to github.com:80, RTT=6.98ms
    Probe 5: Connected to github.com:80, RTT=6.43ms
    Probe 6: Connected to github.com:80, RTT=6.19ms
    Probe 7: Connected to github.com:80, RTT=5.08ms
    Probe 8: Connected to github.com:80, RTT=4.88ms
    Probe 9: Connected to github.com:80, RTT=5.19ms
    Probe 10: Connected to github.com:80, RTT=6.21ms

    Probes sent: 10
    Successful responses: 10
    % of requests failed: 0
    Min response time: 4.8831ms
    Average response time: 5.81053ms
    Median response time: 5.7825ms
    Max response time: 6.9759ms
    Std deviation: 677.203µs
    Jitter (mean abs delta): 790.133µs

    90% of requests were faster than: 6.48558ms
    75% of requests were faster than: 6.36565ms
    50% of requests were faster than: 5.7825ms
    25% of requests were faster than: 5.230975ms

#### Concurrent probes with a histogram

    D:\gotcping> .\gotcping.exe -host github.com -port 443 -count 20 -P 8 -i 0.1 -hist
    Probe 1: Connected to github.com:443, RTT=6.01ms
    ...
    Probe 20: Connected to github.com:443, RTT=5.82ms

    Probes sent: 20
    Successful responses: 20
    % of requests failed: 0
    ...
    25% of requests were faster than: 5.71ms
    95% of requests were faster than: 6.41ms
    99% of requests were faster than: 6.57ms

    RTT histogram (ms):
       5.71 -    5.75 | ######## (4)
       5.75 -    5.79 | ###### (3)
       ...

#### TLS handshake timing

    D:\gotcping> .\gotcping.exe -host api.github.com -port 443 -count 3 -tls -q
    Probes sent: 3
    Successful responses: 3
    % of requests failed: 0
    Min response time: 14.21ms
    Average response time: 15.06ms
    Median response time: 15.10ms
    Max response time: 15.88ms
    Std deviation: 683.1µs
    Jitter (mean abs delta): 712.5µs

    90% of requests were faster than: 15.72ms
    ...

Each probe line also reports the TLS handshake time:

    Probe 2: Connected to api.github.com:443, RTT=14.98ms, TLS handshake=22.31ms

#### Kernel RTT from TCP_INFO (Linux)

    D:\gotcping> .\gotcping.exe -host github.com -port 443 -count 5 -rtt
    Probe 1: Connected to github.com:443, RTT=5.98ms
    ...

    Probes sent: 5
    Successful responses: 5
    ...
    Average kernel RTT: 5.12ms

#### Machine-readable output: `-json` (stdout only)

Emits a single JSON object — ideal for feeding dashboards, cron checks, or scraping. Per-probe lines and failures go to stderr, so stdout stays pure JSON.

    D:\gotcping> .\gotcping.exe -host github.com -count 3 -port 443 -json
    {
      "host": "github.com",
      "port": 443,
      "probes_sent": 3,
      "successful": 3,
      "failed": 0,
      "percent_failed": 0,
      "min_ms": 5.4834,
      "avg_ms": 6.2611,
      "median_ms": 6.6021,
      "max_ms": 6.6978,
      "stddev_ms": 0.5513030564036446,
      "jitter_ms": 0.6550500000000001,
      "p25_ms": 6.04275,
      "p50_ms": 6.6021,
      "p75_ms": 6.6499500000000005,
      "p90_ms": 6.67866,
      "p95_ms": 6.6942,
      "p99_ms": 6.6972,
      "kernel_rtt_avg_ms": 0
    }

A one-liner to grab just the median RTT in milliseconds:

    gotcping -host github.com -count 10 -port 443 -json | jq .median_ms

#### Streaming output: `-jsonl` (one JSON object per line)

    $ gotcping -host github.com -port 443 -count 2 -jsonl
    {"event":"probe","number":1,"ok":true,"rtt_ms":5.91,"ts":"2026-08-09T16:54:37.85Z"}
    {"event":"probe","number":2,"ok":true,"rtt_ms":6.44,"ts":"2026-08-09T16:54:37.90Z"}
    {"event":"summary","host":"github.com","port":443,"probes_sent":2,"successful":2,"failed":0,"percent_failed":0,"min_ms":5.91,"avg_ms":6.18,"median_ms":6.18,"max_ms":6.44,"stddev_ms":0.27,"jitter_ms":0.53,"p25_ms":6.04,"p50_ms":6.18,"p75_ms":6.31,"p90_ms":6.39,"p95_ms":6.42,"p99_ms":6.44,"kernel_rtt_avg_ms":0}

#### Tabular output: `-csv`

    $ gotcping -host github.com -port 443 -count 2 -csv
    event,probe,ok,rtt_ms,tls_ms,kernel_rtt_ms,error
    probe,1,true,5.910,,
    probe,2,true,6.440,,

    event,host,port,probes_sent,successful,failed,percent_failed,min_ms,avg_ms,median_ms,max_ms,stddev_ms,jitter_ms,p25_ms,p50_ms,p75_ms,p90_ms,p95_ms,p99_ms,kernel_rtt_avg_ms
    summary,github.com,443,2,2,0,0.0000,5.9100,6.1750,6.1750,6.4400,0.2650,0.5300,6.0425,6.1750,6.3075,6.3885,6.4143,6.4348,0.0000

#### Probing multiple targets from a file

    $ cat targets.txt
    # web servers
    example.com
    github.com:443
    [::1]:8443
    $ gotcping -file targets.txt -count 3
    == example.com:80 ==
    Probe 1: Connected to example.com:80, RTT=5.54ms
    ...
    Probes sent: 3
    Successful responses: 3
    ...

    == github.com:443 ==
    Probe 1: Connected to github.com:443, RTT=6.12ms
    ...

The exit code is the most severe across all targets.

#### Alerting on loss: `-exit-loss`

    $ gotcping -host 10.0.0.5 -count 10 -exit-loss 5
    Failed to connect to 10.0.0.5 on port 80: dial tcp 10.0.0.5:80: i/o timeout
    ...
    % of requests failed: 40
    ...
    (exit code 4)

#### Print version

    D:\gotcping> .\gotcping.exe -version
    gotcping 0.7.0

#### Stop on a deadline (overrides a large `-count`)

    D:\gotcping> .\gotcping.exe -host example.com -count 50 -deadline 3 -timeout 2
    Probe 1: Connected to example.com:80, RTT=5.54ms
    Probe 2: Connected to example.com:80, RTT=5.73ms
    Probe 3: Connected to example.com:80, RTT=7.75ms

    Probes sent: 3
    Successful responses: 3
    % of requests failed: 0
    Min response time: 5.54ms
    Average response time: 6.340333ms
    Median response time: 5.7276ms
    Max response time: 7.7534ms
    Std deviation: 1.002119ms
    Jitter (mean abs delta): 1.1067ms

    90% of requests were faster than: 7.34824ms
    75% of requests were faster than: 6.7405ms
    50% of requests were faster than: 5.7276ms
    25% of requests were faster than: 5.6338ms

#### All probes fail (non-zero exit code, real error reported)

    D:\gotcping> .\gotcping.exe -host 127.0.0.1 -port 1 -count 2 -timeout 1
    Failed to connect to 127.0.0.1 on port 1: dial tcp 127.0.0.1:1: connectex: No connection could be made because the target machine actively refused it.
    Failed to connect to 127.0.0.1 on port 1: dial tcp 127.0.0.1:1: connectex: No connection could be made because the target machine actively refused it.

    All the requests have failed. The host 127.0.0.1 is not replying to connections on 1

    (exit code 3)

#### Quiet summary with a fast interval: `-q -i`

    D:\gotcping> .\gotcping.exe -host github.com -count 3 -port 443 -q -i 0.2

    Probes sent: 3
    Successful responses: 3
    % of requests failed: 0
    Min response time: 5.7146ms
    Average response time: 5.973733ms
    Median response time: 5.8275ms
    Max response time: 6.3791ms
    Std deviation: 290.319µs
    Jitter (mean abs delta): 332.25µs

    90% of requests were faster than: 6.26878ms
    75% of requests were faster than: 6.1033ms
    50% of requests were faster than: 5.8275ms
    25% of requests were faster than: 5.77105ms

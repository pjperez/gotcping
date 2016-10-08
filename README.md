# TCPing
Yet another tcping tool - forked from https://github.com/hwsdien/gotcping

Measure your RTT / latency to any TCP endpoint.

## Requirements

gotcping uses the awesome [stats](https://github.com/montanaflynn/stats)  library by [montanaflynn](https://github.com/montanaflynn) 

## Installing
### From source

    go get github.com/pjperez/gotcping

## Binaries

### Windows

[gotcping 0.5](https://github.com/pjperez/gotcping/releases/tag/0.5)

## Usage

    gotcping -host hostname [-port port_number] [-count number_of_repetitions] [-timeout timeout_in_seconds]

### Defaults

        -port default to 80
        -count defaults to 10
        -timeout defaults to 1 

### Example

    D:\gotcping> .\gotcping.exe -host github.com -count 5 -port 443
    Connected to github.com:443, RTT=93.91ms
    Connected to github.com:443, RTT=93.31ms
    Connected to github.com:443, RTT=98.64ms
    Connected to github.com:443, RTT=100.04ms
    Connected to github.com:443, RTT=89.22ms
    

    Probes sent: 5
    Successful responses: 5
    % of requests failed: 0
    Min response time: 89.2242ms
    Average response time: 95.02352ms
    Median response time: 93.9077ms
    Max response time: 100.0405ms

    90% of requests were faster than: 100.0405ms
    75% of requests were faster than: 98.6356ms
    50% of requests were faster than: 93.9077ms
    25% of requests were faster than: 89.2242ms


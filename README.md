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

    gotcping -host host [-port port_number] [-count number_of_repetitions] [-timeout timeout_in_seconds]

### Alternate (only host is mandatory and can be specified either as a flag or as an argument)

    gotcping host

### Defaults

        -port default to 80
        -count defaults to 10 probes
        -timeout defaults to 1 second

### Example

#### Specify all parameters

    D:\gotcping> .\gotcping.exe -host github.com -count 5 -port 443
    Probe 1: Connected to github.com:443, RTT=84.81ms
    Probe 2: Connected to github.com:443, RTT=88.73ms
    Probe 3: Connected to github.com:443, RTT=83.29ms
    Probe 4: Connected to github.com:443, RTT=88.43ms
    Probe 5: Connected to github.com:443, RTT=83.65ms

    Probes sent: 5
    Successful responses: 5
    % of requests failed: 0
    Min response time: 83.2916ms
    Average response time: 85.78024ms
    Median response time: 84.8074ms
    Max response time: 88.7265ms

    90% of requests were faster than: 88.7265ms
    75% of requests were faster than: 88.4301ms
    50% of requests were faster than: 84.8074ms
    25% of requests were faster than: 83.2916ms

#### Specify only host

    D:\gotcping> .\gotcping.exe github.com
    Probe 1: Connected to github.com:80, RTT=82.76ms
    Probe 2: Connected to github.com:80, RTT=84.10ms
    Probe 3: Connected to github.com:80, RTT=83.69ms
    Probe 4: Connected to github.com:80, RTT=82.61ms
    Probe 5: Connected to github.com:80, RTT=84.19ms
    Probe 6: Connected to github.com:80, RTT=81.33ms
    Probe 7: Connected to github.com:80, RTT=84.50ms
    Probe 8: Connected to github.com:80, RTT=87.60ms
    Probe 9: Connected to github.com:80, RTT=83.36ms
    Probe 10: Connected to github.com:80, RTT=84.28ms

    Probes sent: 10
    Successful responses: 10
    % of requests failed: 0
    Min response time: 81.3263ms
    Average response time: 83.84194ms
    Median response time: 83.8931ms
    Max response time: 87.6028ms

    90% of requests were faster than: 86.05115ms
    75% of requests were faster than: 84.2835ms
    50% of requests were faster than: 83.8931ms
    25% of requests were faster than: 82.7575ms
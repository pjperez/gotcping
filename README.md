# TCPing
Yet another tcping tool - forked from https://github.com/hwsdien/gotcping

Measure your TCP latency to any TCP endpoint.

## Requirements

Golang > 1.3

## Installing
### From source

    go get github.com/pjperez/gotcping

## Usage

    gotcping -host hostname [-port port_number] [-count number_of_repetitions] [-timeout timeout_in_seconds]

### Defaults

        -host defaults to bing.com
        -port default to 80
        -count defaults to 10
        -timeout defaults to 5 

### Example

    D:\gotcping> gotcping -host github.com -port 80 -count 5 -timeout 1
    Connected to github.com:80, RTT=4.04ms
    Connected to github.com:80, RTT=3.95ms
    Connected to github.com:80, RTT=6.01ms
    Connected to github.com:80, RTT=4.56ms
    Connected to github.com:80, RTT=3.04ms
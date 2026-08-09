package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// target is one host[:port] entry from a -file list. port is 0 when the line
// did not specify one (the run-level default applies).
type target struct {
	host string
	port int
}

// readTargets parses a host file: one host[:port] per line, blank lines and
// '#' comments ignored. Ports must be 1-65535.
func readTargets(path string) ([]target, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("can't open target file: %w", err)
	}
	defer f.Close()

	var targets []target
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		t, err := parseTargetLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		targets = append(targets, t)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading %s: %w", path, err)
	}
	return targets, nil
}

// parseTargetLine splits "host[:port]" (including bracketed IPv6 literals).
// A line with no port is accepted as the bare host.
func parseTargetLine(line string) (target, error) {
	host, portStr, err := net.SplitHostPort(line)
	if err != nil {
		return target{host: line}, nil
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return target{}, fmt.Errorf("invalid port %q in target %q", portStr, line)
	}
	return target{host: host, port: p}, nil
}

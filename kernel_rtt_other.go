//go:build !linux

package main

import (
	"net"
	"time"
)

// kernelRTT returns the kernel-smoothed RTT for conn. Platforms without
// TCP_INFO support report false (no kernel RTT is surfaced).
func kernelRTT(net.Conn) (time.Duration, bool) {
	return 0, false
}

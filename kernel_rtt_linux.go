//go:build linux

package main

import (
	"net"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// kernelRTT reads the kernel-smoothed RTT (tcpi_rtt, microseconds) of an
// established TCP connection via getsockopt(TCP_INFO). It reports false when
// the connection does not expose a raw socket.
func kernelRTT(conn net.Conn) (time.Duration, bool) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return 0, false
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return 0, false
	}
	var rtt time.Duration
	var got bool
	if err := raw.Control(func(fd uintptr) {
		info, err := unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
		if err != nil {
			return
		}
		rtt = time.Duration(info.Rtt) * time.Microsecond
		got = true
	}); err != nil {
		return 0, false
	}
	return rtt, got
}

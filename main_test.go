package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidatePort(t *testing.T) {
	cases := []struct {
		port int
		ok   bool
	}{
		{0, false}, {-1, false}, {1, true}, {80, true},
		{65535, true}, {65536, false}, {100000, false},
	}
	for _, c := range cases {
		err := validatePort(c.port)
		if c.ok && err != nil {
			t.Errorf("validatePort(%d): expected ok, got %v", c.port, err)
		}
		if !c.ok && err == nil {
			t.Errorf("validatePort(%d): expected error, got nil", c.port)
		}
	}
}

func TestValidateTimeout(t *testing.T) {
	cases := []struct {
		t  float64
		ok bool
	}{
		{-5, false}, {0, false}, {0.0009, false}, {0.001, true},
		{0.2, true}, {1, true}, {5, true},
	}
	for _, c := range cases {
		_, err := validateTimeout(c.t)
		if c.ok && err != nil {
			t.Errorf("validateTimeout(%v): expected ok, got %v", c.t, err)
		}
		if !c.ok && err == nil {
			t.Errorf("validateTimeout(%v): expected error, got nil", c.t)
		}
	}
	// Fractional seconds must convert to a millisecond-precision Duration.
	d, err := validateTimeout(0.2)
	if err != nil || d != 200*time.Millisecond {
		t.Errorf("validateTimeout(0.2) = %v, %v, want 200ms nil", d, err)
	}
}

func TestValidateDeadline(t *testing.T) {
	if err := validateDeadline(-1); err == nil {
		t.Error("validateDeadline(-1): expected error")
	}
	if err := validateDeadline(0); err != nil {
		t.Errorf("validateDeadline(0): expected ok, got %v", err)
	}
	if err := validateDeadline(60); err != nil {
		t.Errorf("validateDeadline(60): expected ok, got %v", err)
	}
}

func TestResolveFamily(t *testing.T) {
	cases := []struct {
		v4, v6 bool
		want   string
		ok     bool
	}{
		{false, false, "", true},
		{true, false, "ip4", true},
		{false, true, "ip6", true},
		{true, true, "", false}, // mutually exclusive
	}
	for _, c := range cases {
		got, err := resolveFamily(c.v4, c.v6)
		if c.ok {
			if err != nil {
				t.Errorf("resolveFamily(%v,%v): expected ok, got %v", c.v4, c.v6, err)
				continue
			}
			if got != c.want {
				t.Errorf("resolveFamily(%v,%v) = %q, want %q", c.v4, c.v6, got, c.want)
			}
		} else if err == nil {
			t.Errorf("resolveFamily(%v,%v): expected error, got %q", c.v4, c.v6, got)
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"example.com", "example.com"},
		{"\x1b[2J\rexample.com", "?[2J?example.com"},
		{"a\tb\nc", "a?b?c"},
		{"127.0.0.1", "127.0.0.1"},
		{"del\x7fete", "del?ete"},
	}
	for _, c := range cases {
		if got := sanitize(c.in); got != c.want {
			t.Errorf("sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSampleRingUnderCap(t *testing.T) {
	r := newSampleRing(5)
	for _, v := range []float64{1, 2, 3} {
		r.add(v)
	}
	got := r.values()
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("under-cap values = %v, want [1 2 3]", got)
	}
	if r.full {
		t.Error("ring should not be full under cap")
	}
}

func TestSampleRingOverflow(t *testing.T) {
	r := newSampleRing(3)
	for _, v := range []float64{1, 2, 3, 4, 5} {
		r.add(v)
	}
	got := r.values()
	// After 5 adds into cap 3, the window is [3, 4, 5] (oldest evicted).
	if len(got) != 3 || got[0] != 3 || got[1] != 4 || got[2] != 5 {
		t.Errorf("overflow values = %v, want [3 4 5]", got)
	}
	if !r.full {
		t.Error("ring should be full after overflow")
	}
}

func TestSampleRingCapGuard(t *testing.T) {
	r := newSampleRing(0) // must be coerced to >= 1
	r.add(42)
	if got := r.values(); len(got) != 1 || got[0] != 42 {
		t.Errorf("cap-guard values = %v, want [42]", got)
	}
}

func TestComputeStatsBasic(t *testing.T) {
	// 1ms, 2ms, 3ms (in ns).
	samples := []float64{1e6, 2e6, 3e6}
	s, err := computeStats(3, 3, samples, nil, "h", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.MinMs != 1 || s.MaxMs != 3 || s.AvgMs != 2 {
		t.Errorf("min/avg/max = %v/%v/%v, want 1/2/3", s.MinMs, s.AvgMs, s.MaxMs)
	}
	if s.MedianMs != 2 {
		t.Errorf("median = %v, want 2", s.MedianMs)
	}
	if s.P50Ms != s.MedianMs {
		t.Errorf("p50 (%v) should equal median (%v)", s.P50Ms, s.MedianMs)
	}
	if s.Failed != 0 || s.PercentFailed != 0 {
		t.Errorf("failed/percent = %v/%v, want 0/0", s.Failed, s.PercentFailed)
	}
	// p95/p99 must exist and be sane for [1,2,3]ms.
	if s.P95Ms < s.P90Ms || s.P99Ms < s.P95Ms {
		t.Errorf("percentiles not monotonic: p90=%v p95=%v p99=%v", s.P90Ms, s.P95Ms, s.P99Ms)
	}
}

func TestComputeStatsJitter(t *testing.T) {
	// Deltas: |2-1|=1, |5-2|=3, |5-5|=0 => mean = 4/3.
	samples := []float64{1e6, 2e6, 5e6, 5e6}
	s, err := computeStats(4, 4, samples, nil, "h", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantJitter := (1.0 + 3.0 + 0.0) / 3.0
	if s.JitterMs != wantJitter {
		t.Errorf("jitter = %v, want %v", s.JitterMs, wantJitter)
	}
	// Single sample: jitter must be 0 (no consecutive pair).
	s1, _ := computeStats(1, 1, []float64{5e6}, nil, "h", 80)
	if s1.JitterMs != 0 {
		t.Errorf("jitter for single sample = %v, want 0", s1.JitterMs)
	}
}

func TestComputeStatsFailureAccounting(t *testing.T) {
	// 5 attempts, 3 successes => 2 failed, 40%.
	s, err := computeStats(5, 3, []float64{1e6, 2e6, 3e6}, nil, "h", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Failed != 2 || s.PercentFailed != 40 {
		t.Errorf("failed/percent = %v/%v, want 2/40", s.Failed, s.PercentFailed)
	}
}

func TestComputeStatsNoSuccess(t *testing.T) {
	if _, err := computeStats(3, 0, nil, nil, "h", 80); err == nil {
		t.Error("expected error when no successful probes")
	}
}

func TestComputeStatsKernelSamples(t *testing.T) {
	// Kernel RTTs in ns: 1.5ms and 2.5ms => avg 2ms.
	s, err := computeStats(2, 2, []float64{1e6, 2e6}, []float64{1.5e6, 2.5e6}, "h", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.KernelRttAvgMs != 2 {
		t.Errorf("kernel_rtt_avg_ms = %v, want 2", s.KernelRttAvgMs)
	}
	s2, _ := computeStats(2, 2, []float64{1e6, 2e6}, nil, "h", 80)
	if s2.KernelRttAvgMs != 0 {
		t.Errorf("kernel_rtt_avg_ms without samples = %v, want 0", s2.KernelRttAvgMs)
	}
}

func TestComputeStatsJSON(t *testing.T) {
	s, _ := computeStats(2, 2, []float64{1e6, 2e6}, nil, "example.com", 443)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Round-trip to ensure all fields are valid JSON and types.
	var back statistics
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Host != "example.com" || back.Port != 443 || back.Successful != 2 {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestValidateInterval(t *testing.T) {
	if _, err := validateInterval(0); err == nil {
		t.Error("validateInterval(0): expected error")
	}
	if _, err := validateInterval(-1); err == nil {
		t.Error("validateInterval(-1): expected error")
	}
	d, err := validateInterval(0.5)
	if err != nil || d != 500*time.Millisecond {
		t.Errorf("validateInterval(0.5) = %v, %v, want 500ms nil", d, err)
	}
}

func TestParseSource(t *testing.T) {
	ip, err := parseSource("127.0.0.1")
	if err != nil || !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("parseSource(127.0.0.1) = %v, %v, want 127.0.0.1 nil", ip, err)
	}
	if _, err := parseSource("no.such.iface.xyz"); err == nil {
		t.Error("parseSource(bogus): expected error")
	}
}

func TestParsePortRange(t *testing.T) {
	cases := []struct {
		in         string
		start, end int
		ok         bool
	}{
		{"", 0, 0, true}, // ephemeral
		{"1024", 1024, 1024, true},
		{"2000-3000", 2000, 3000, true},
		{"1-65535", 1, 65535, true},
		{"0", 0, 0, false},
		{"65536", 0, 0, false},
		{"3000-2000", 0, 0, false},
		{"abc", 0, 0, false},
		{"1-2-3", 0, 0, false},
	}
	for _, c := range cases {
		start, end, err := parsePortRange(c.in)
		if c.ok {
			if err != nil {
				t.Errorf("parsePortRange(%q): expected ok, got %v", c.in, err)
				continue
			}
			if start != c.start || end != c.end {
				t.Errorf("parsePortRange(%q) = %d-%d, want %d-%d", c.in, start, end, c.start, c.end)
			}
		} else if err == nil {
			t.Errorf("parsePortRange(%q): expected error, got %d-%d", c.in, start, end)
		}
	}
}

func TestParseTargetLine(t *testing.T) {
	cases := []struct {
		in   string
		host string
		port int
	}{
		{"example.com", "example.com", 0},
		{"example.com:443", "example.com", 443},
		{"::1", "::1", 0},
		{"[::1]:8443", "::1", 8443},
	}
	for _, c := range cases {
		got, err := parseTargetLine(c.in)
		if err != nil {
			t.Errorf("parseTargetLine(%q): unexpected error %v", c.in, err)
			continue
		}
		if got.host != c.host || got.port != c.port {
			t.Errorf("parseTargetLine(%q) = %q:%d, want %q:%d", c.in, got.host, got.port, c.host, c.port)
		}
	}
	if _, err := parseTargetLine("example.com:99999"); err == nil {
		t.Error("parseTargetLine(bad port): expected error")
	}
	if _, err := parseTargetLine("example.com:notaport"); err == nil {
		t.Error("parseTargetLine(non-numeric port): expected error")
	}
}

func TestReadTargets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.txt")
	content := "# comment\n\nexample.com\nexample.com:443\n[::1]:8443\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	targets, err := readTargets(path)
	if err != nil {
		t.Fatalf("readTargets: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("got %d targets, want 3: %+v", len(targets), targets)
	}
	if targets[1].host != "example.com" || targets[1].port != 443 {
		t.Errorf("target[1] = %+v, want example.com:443", targets[1])
	}
	if targets[2].host != "::1" || targets[2].port != 8443 {
		t.Errorf("target[2] = %+v, want [::1]:8443", targets[2])
	}
	if _, err := readTargets(filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("readTargets(missing): expected error")
	}
}

func TestComputeHistogram(t *testing.T) {
	if got := computeHistogram(nil, 10); got != nil {
		t.Errorf("computeHistogram(nil) = %v, want nil", got)
	}
	ms := []float64{1, 2, 3, 4, 5}
	bins := computeHistogram(ms, 4)
	if len(bins) != 4 {
		t.Fatalf("got %d bins, want 4", len(bins))
	}
	total := 0
	for _, b := range bins {
		total += b.Count
		if b.Hi < b.Lo {
			t.Errorf("bin inverted: %+v", b)
		}
	}
	if total != len(ms) {
		t.Errorf("bin counts sum to %d, want %d", total, len(ms))
	}
}

// fakeConn is a net.Conn that does not expose a raw syscall.Conn, so kernelRTT
// must report "unavailable" on every platform.
type fakeConn struct{ net.Conn }

func TestKernelRTTUnavailable(t *testing.T) {
	if rtt, ok := kernelRTT(fakeConn{}); ok {
		t.Errorf("kernelRTT(fakeConn) = %v, ok=true; want unavailable", rtt)
	}
}

func TestPingLocalTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	opts := options{
		count:       3,
		timeout:     time.Second,
		interval:    10 * time.Millisecond,
		concurrency: 1,
		quiet:       true,
	}
	if code := ping("127.0.0.1", "127.0.0.1", port, opts); code != exitOK {
		t.Errorf("ping local TCP: exit code %d, want 0", code)
	}
}

func TestPingTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	opts := options{
		count:       2,
		timeout:     2 * time.Second,
		interval:    10 * time.Millisecond,
		concurrency: 1,
		quiet:       true,
		sni:         "example.com",
		insecureTLS: true,
	}
	if code := ping("127.0.0.1", "127.0.0.1", port, opts); code != exitOK {
		t.Errorf("ping TLS: exit code %d, want 0", code)
	}
}

func TestEmitSummaryExitCodes(t *testing.T) {
	// No successful probes => exitAllFailed regardless of threshold.
	if code := emitSummary(3, 0, nil, nil, "h", 80, options{quiet: true, exitLoss: -1}, nil); code != exitAllFailed {
		t.Errorf("all-failed: got %d, want %d", code, exitAllFailed)
	}
	// 3 attempts, 2 successes => 33.3% failed > threshold 10 => exitLossExceeded.
	if code := emitSummary(3, 2, []float64{1e6, 2e6}, nil, "h", 80, options{quiet: true, exitLoss: 10}, nil); code != exitLossExceeded {
		t.Errorf("loss exceeded: got %d, want %d", code, exitLossExceeded)
	}
	// Threshold 50 covers a 33.3% failure rate => exitOK.
	if code := emitSummary(3, 2, []float64{1e6, 2e6}, nil, "h", 80, options{quiet: true, exitLoss: 50}, nil); code != exitOK {
		t.Errorf("loss ok: got %d, want %d", code, exitOK)
	}
}

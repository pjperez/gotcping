package main

import (
	"encoding/json"
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
		t  int
		ok bool
	}{
		{-5, false}, {0, false}, {1, true}, {5, true},
	}
	for _, c := range cases {
		err := validateTimeout(c.t)
		if c.ok && err != nil {
			t.Errorf("validateTimeout(%d): expected ok, got %v", c.t, err)
		}
		if !c.ok && err == nil {
			t.Errorf("validateTimeout(%d): expected error, got nil", c.t)
		}
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
	s, err := computeStats(3, 3, samples, "h", 80)
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
}

func TestComputeStatsJitter(t *testing.T) {
	// Deltas: |2-1|=1, |5-2|=3, |5-5|=0 => mean = 4/3.
	samples := []float64{1e6, 2e6, 5e6, 5e6}
	s, err := computeStats(4, 4, samples, "h", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantJitter := (1.0 + 3.0 + 0.0) / 3.0
	if s.JitterMs != wantJitter {
		t.Errorf("jitter = %v, want %v", s.JitterMs, wantJitter)
	}
	// Single sample: jitter must be 0 (no consecutive pair).
	s1, _ := computeStats(1, 1, []float64{5e6}, "h", 80)
	if s1.JitterMs != 0 {
		t.Errorf("jitter for single sample = %v, want 0", s1.JitterMs)
	}
}

func TestComputeStatsFailureAccounting(t *testing.T) {
	// 5 attempts, 3 successes => 2 failed, 40%.
	s, err := computeStats(5, 3, []float64{1e6, 2e6, 3e6}, "h", 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Failed != 2 || s.PercentFailed != 40 {
		t.Errorf("failed/percent = %v/%v, want 2/40", s.Failed, s.PercentFailed)
	}
}

func TestComputeStatsNoSuccess(t *testing.T) {
	if _, err := computeStats(3, 0, nil, "h", 80); err == nil {
		t.Error("expected error when no successful probes")
	}
}

func TestComputeStatsJSON(t *testing.T) {
	s, _ := computeStats(2, 2, []float64{1e6, 2e6}, "example.com", 443)
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

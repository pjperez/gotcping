package main

import (
	"testing"
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

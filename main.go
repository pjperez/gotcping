package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/montanaflynn/stats"
)

// Exit codes (kept stable for scripts/CI wrapping this tool).
const (
	exitOK            = 0
	exitUsage         = 1
	exitResolveFailed = 2
	exitAllFailed     = 3
)

// Version is the current version of gotcping. It defaults to "dev" for
// source builds and is overridden at release time via ldflags, e.g.
//
//	go build -ldflags="-X main.Version=0.6.0" .
var Version = "dev"

// maxSamples caps how many RTT samples are retained for statistics. In
// infinite mode this bounds memory; for typical finite runs it is never hit.
const maxSamples = 100000

func main() {
	hostPtr := flag.String("host", "", "Host or IP address to test")
	portPtr := flag.Int("port", 80, "Port number to query (1-65535)")
	countPtr := flag.Int("count", 10, "Number of requests to send [0 or negative means infinite]")
	timeoutPtr := flag.Int("timeout", 1, "Timeout for each request, in seconds (>=1)")
	deadlinePtr := flag.Int("deadline", 0, "Stop after this many seconds regardless of count [0 means no deadline]")
	intervalPtr := flag.Float64("i", 1.0, "Seconds between probes (>= 0.001)")
	quietPtr := flag.Bool("q", false, "Quiet: suppress per-probe lines, print only the summary")
	jsonPtr := flag.Bool("json", false, "Emit results as a single JSON object on stdout (for automation)")
	ipv4OnlyPtr := flag.Bool("4", false, "Force IPv4 when resolving the host")
	ipv6OnlyPtr := flag.Bool("6", false, "Force IPv6 when resolving the host")
	versionPtr := flag.Bool("version", false, "Print version and exit")

	flag.Parse()

	if *versionPtr {
		fmt.Println("gotcping", Version)
		os.Exit(exitOK)
	}

	// Accept a bare positional host argument: `gotcping example.com`.
	host := *hostPtr
	if host == "" && len(os.Args) == 2 {
		arg := os.Args[1]
		// Guard against empty/flag-like args without slicing an empty string.
		if arg != "" && !strings.HasPrefix(arg, "-") {
			host = arg
		}
	}

	port := *portPtr
	count := *countPtr
	timeout := *timeoutPtr
	deadline := *deadlinePtr

	if host == "" {
		flag.Usage()
		os.Exit(exitUsage)
	}
	if err := validatePort(port); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	if err := validateTimeout(timeout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	if err := validateDeadline(deadline); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	interval, err := validateInterval(*intervalPtr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	family, err := resolveFamily(*ipv4OnlyPtr, *ipv6OnlyPtr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}

	// Resolve once and dial the resulting IP, so the host we validate is the
	// host we actually connect to (avoids DNS-rebinding TOCTOU between a
	// validation lookup and the dial, and re-resolution per probe).
	resolvedIP, err := resolveHost(host, family)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(exitResolveFailed)
	}

	opts := options{
		count:    count,
		timeout:  timeout,
		deadline: deadline,
		interval: interval,
		quiet:    *quietPtr,
		json:     *jsonPtr,
	}

	ping(sanitize(host), resolvedIP, port, opts)
}

// validatePort returns an error if p is outside the valid TCP port range.
func validatePort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port must be between 1 and 65535 (got %d)", p)
	}
	return nil
}

// validateTimeout enforces a positive dial timeout. A non-positive timeout
// disables the dial timeout on most platforms, defeating the tool's own DoS
// protection and hanging the process.
func validateTimeout(t int) error {
	if t < 1 {
		return fmt.Errorf("timeout must be at least 1 second (got %d)", t)
	}
	return nil
}

// validateDeadline enforces a non-negative deadline.
func validateDeadline(d int) error {
	if d < 0 {
		return fmt.Errorf("deadline must be >= 0 seconds (got %d)", d)
	}
	return nil
}

// validateInterval enforces a positive, sub-millisecond-floor inter-probe
// interval and returns it as a Duration.
func validateInterval(seconds float64) (time.Duration, error) {
	if seconds < 0.001 {
		return 0, fmt.Errorf("interval must be >= 0.001 seconds (got %g)", seconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// options configures a ping run.
type options struct {
	count    int
	timeout  int
	deadline int
	interval time.Duration
	quiet    bool
	json     bool
}

// resolveFamily maps the -4/-6 flags to a net IP-network family. "" (any)
// means no filter. Specifying both flags is an error.
func resolveFamily(ipv4Only, ipv6Only bool) (string, error) {
	if ipv4Only && ipv6Only {
		return "", fmt.Errorf("-4 and -6 are mutually exclusive")
	}
	if ipv4Only {
		return "ip4", nil
	}
	if ipv6Only {
		return "ip6", nil
	}
	return "", nil
}

// resolveHost resolves host to a single IP string (the first result matching
// family). An empty family accepts any address family.
func resolveHost(host, family string) (string, error) {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("can't resolve %s", host)
	}
	if family == "" {
		return ips[0].String(), nil
	}
	for _, ip := range ips {
		switch family {
		case "ip4":
			if v4 := ip.To4(); v4 != nil {
				return v4.String(), nil
			}
		case "ip6":
			// To4() is non-nil for 4-in-6 mapped addresses; treat those as v4.
			if ip.To4() == nil {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no %s address found for %s", family, host)
}

// sanitize replaces terminal control characters with '?' so an attacker
// controlled hostname cannot forge log lines or inject ANSI sequences into
// the tool's output.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '?'
		}
		return r
	}, s)
}

func ping(displayHost, resolvedIP string, port int, opts options) {
	attempts := 0
	successfulProbes := 0
	ring := newSampleRing(maxSamples)

	// net.JoinHostPort correctly brackets IPv6 literals (also clears the
	// `go vet` "address format does not work with IPv6" warning).
	addr := net.JoinHostPort(resolvedIP, strconv.Itoa(port))

	// Always allow Ctrl+C / SIGTERM (or a deadline) to stop and still print
	// results, for both finite and infinite runs.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	var deadlineCh <-chan time.Time
	if opts.deadline > 0 {
		deadlineCh = time.After(time.Duration(opts.deadline) * time.Second)
	}

	// Per-probe diagnostics (success lines, failure lines) are suppressed in
	// quiet/json mode and on stderr otherwise, so the summary/stdout stays a
	// clean, parseable stream.
	showProbe := !opts.quiet && !opts.json

loop:
	for {
		if opts.count >= 1 && attempts >= opts.count {
			break
		}
		select {
		case <-stop:
			break loop
		case <-deadlineCh:
			break loop
		default:
		}

		attempts++
		timeStart := time.Now()
		_, err := net.DialTimeout("tcp", addr, time.Second*time.Duration(opts.timeout))
		responseTime := time.Since(timeStart)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to %s on port %d: %v\n", displayHost, port, err)
		} else {
			if showProbe {
				fmt.Printf("Probe %v: Connected to %s:%d, RTT=%.2fms\n", attempts, displayHost, port, float64(responseTime.Nanoseconds())/1e6)
			}
			successfulProbes++
			ring.add(float64(responseTime))
		}

		// Don't sleep after the last probe of a finite run, so results are
		// displayed ~one interval sooner.
		if opts.count < 1 || attempts < opts.count {
			sleep := opts.interval - responseTime
			if sleep < 0 {
				sleep = 0
			}
			select {
			case <-stop:
				break loop
			case <-deadlineCh:
				break loop
			case <-time.After(sleep):
			}
		}
	}

	if err := output(attempts, successfulProbes, ring.values(), displayHost, port, opts); err != nil {
		// All probes failed.
		if opts.json {
			fmt.Fprintf(os.Stderr, "{\"error\":%q,\"host\":%q,\"port\":%d}\n", err.Error(), displayHost, port)
		} else {
			fmt.Fprintf(os.Stderr, "\nAll the requests have failed. The host %s is not replying to connections on %d\n", displayHost, port)
		}
		os.Exit(exitAllFailed)
	}
}

// sampleRing is a fixed-capacity ring buffer of float64 samples. Once full,
// new samples overwrite the oldest. values() returns the current contents in
// insertion order.
type sampleRing struct {
	buf  []float64
	cap  int
	head int
	full bool
}

func newSampleRing(c int) *sampleRing {
	if c < 1 {
		c = 1
	}
	return &sampleRing{cap: c}
}

func (r *sampleRing) add(v float64) {
	if !r.full {
		if len(r.buf) < r.cap {
			r.buf = append(r.buf, v)
			return
		}
		r.full = true
	}
	r.buf[r.head] = v
	r.head++
	if r.head == r.cap {
		r.head = 0
	}
}

func (r *sampleRing) values() []float64 {
	if !r.full {
		return r.buf
	}
	out := make([]float64, r.cap)
	n := copy(out, r.buf[r.head:])
	copy(out[n:], r.buf[:r.head])
	return out
}

// statistics is the computed summary of a run. All latency fields are in
// milliseconds. It is serialized directly for the -json output mode.
type statistics struct {
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	ProbesSent    int     `json:"probes_sent"`
	Successful    int     `json:"successful"`
	Failed        int     `json:"failed"`
	PercentFailed float64 `json:"percent_failed"`
	MinMs         float64 `json:"min_ms"`
	AvgMs         float64 `json:"avg_ms"`
	MedianMs      float64 `json:"median_ms"`
	MaxMs         float64 `json:"max_ms"`
	StdDevMs      float64 `json:"stddev_ms"`
	JitterMs      float64 `json:"jitter_ms"`
	P25Ms         float64 `json:"p25_ms"`
	P50Ms         float64 `json:"p50_ms"`
	P75Ms         float64 `json:"p75_ms"`
	P90Ms         float64 `json:"p90_ms"`
}

// computeStats summarizes a run. samples are RTTs in nanoseconds as float64
// (one entry per successful probe). It returns an error if there were no
// successful probes.
func computeStats(attempts, successful int, samples []float64, host string, port int) (*statistics, error) {
	if successful == 0 || len(samples) == 0 {
		return nil, errors.New("no successful probes")
	}

	toMs := func(ns float64) float64 { return ns / 1e6 }

	ms := make([]float64, len(samples))
	for i, v := range samples {
		ms[i] = toMs(v)
	}

	sum := 0.0
	min := ms[0]
	max := ms[0]
	for _, v := range ms {
		sum += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	avg := sum / float64(len(ms))

	median, _ := stats.Median(ms)
	stddev, _ := stats.StandardDeviation(ms)
	p25, _ := stats.Percentile(ms, 25)
	p75, _ := stats.Percentile(ms, 75)
	p90, _ := stats.Percentile(ms, 90)

	// Jitter: mean absolute difference between consecutive samples, in time
	// order (a standard packet-delay-variation measure, as in RTP/iperf).
	jitter := 0.0
	if len(ms) > 1 {
		var acc float64
		for i := 1; i < len(ms); i++ {
			d := ms[i] - ms[i-1]
			if d < 0 {
				d = -d
			}
			acc += d
		}
		jitter = acc / float64(len(ms)-1)
	}

	failed := attempts - successful
	percentFailed := 0.0
	if attempts > 0 {
		percentFailed = (float64(failed) * 100) / float64(attempts)
	}

	return &statistics{
		Host:          host,
		Port:          port,
		ProbesSent:    attempts,
		Successful:    successful,
		Failed:        failed,
		PercentFailed: percentFailed,
		MinMs:         min,
		AvgMs:         avg,
		MedianMs:      median,
		MaxMs:         max,
		StdDevMs:      stddev,
		JitterMs:      jitter,
		P25Ms:         p25,
		P50Ms:         median,
		P75Ms:         p75,
		P90Ms:         p90,
	}, nil
}

// output computes stats and renders them (text or JSON) to stdout. It returns
// an error iff every probe failed, so the caller can emit a diagnostic and
// exit non-zero.
func output(attempts, successful int, samples []float64, host string, port int, opts options) error {
	s, err := computeStats(attempts, successful, samples, host, port)
	if err != nil {
		return err
	}
	if opts.json {
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}

	fmt.Println("\nProbes sent:", s.ProbesSent,
		"\nSuccessful responses:", s.Successful,
		"\n% of requests failed:", s.PercentFailed,
		"\nMin response time:", time.Duration(s.MinMs*1e6),
		"\nAverage response time:", time.Duration(s.AvgMs*1e6),
		"\nMedian response time:", time.Duration(s.MedianMs*1e6),
		"\nMax response time:", time.Duration(s.MaxMs*1e6),
		"\nStd deviation:", time.Duration(s.StdDevMs*1e6),
		"\nJitter (mean abs delta):", time.Duration(s.JitterMs*1e6))

	fmt.Println("\n90% of requests were faster than:", time.Duration(s.P90Ms*1e6),
		"\n75% of requests were faster than:", time.Duration(s.P75Ms*1e6),
		"\n50% of requests were faster than:", time.Duration(s.P50Ms*1e6),
		"\n25% of requests were faster than:", time.Duration(s.P25Ms*1e6))
	return nil
}

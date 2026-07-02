package main

import (
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

// maxSamples caps how many RTT samples are retained for statistics. In
// infinite mode this bounds memory; for typical finite runs it is never hit.
const maxSamples = 100000

func main() {
	hostPtr := flag.String("host", "", "Host or IP address to test")
	portPtr := flag.Int("port", 80, "Port number to query (1-65535)")
	countPtr := flag.Int("count", 10, "Number of requests to send [0 or negative means infinite]")
	timeoutPtr := flag.Int("timeout", 1, "Timeout for each request, in seconds (>=1)")
	deadlinePtr := flag.Int("deadline", 0, "Stop after this many seconds regardless of count [0 means no deadline]")

	flag.Parse()

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

	// Resolve once and dial the resulting IP, so the host we validate is the
	// host we actually connect to (avoids DNS-rebinding TOCTOU between a
	// validation lookup and the dial, and re-resolution per probe).
	resolvedIP, err := resolveHost(host)
	if err != nil {
		fmt.Printf("error: can't resolve %s\n", host)
		os.Exit(exitResolveFailed)
	}

	ping(sanitize(host), resolvedIP, port, count, timeout, deadline)
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

// resolveHost resolves host to a single IP string (the first result).
func resolveHost(host string) (string, error) {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("can't resolve %s", host)
	}
	return ips[0].String(), nil
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

func ping(displayHost, resolvedIP string, port int, count int, timeout int, deadline int) {
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
	if deadline > 0 {
		deadlineCh = time.After(time.Duration(deadline) * time.Second)
	}

loop:
	for {
		if count >= 1 && attempts >= count {
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
		_, err := net.DialTimeout("tcp", addr, time.Second*time.Duration(timeout))
		responseTime := time.Since(timeStart)
		if err != nil {
			fmt.Printf("Received timeout while connecting to %s on port %d.\n", displayHost, port)
		} else {
			fmt.Printf("Probe %v: Connected to %s:%d, RTT=%.2fms\n", attempts, displayHost, port, float64(responseTime.Nanoseconds())/1e6)
			successfulProbes++
			ring.add(float64(responseTime))
		}

		// Don't sleep after the last probe of a finite run, so results are
		// displayed ~1 second faster.
		if count < 1 || attempts < count {
			select {
			case <-stop:
				break loop
			case <-deadlineCh:
				break loop
			case <-time.After(time.Second - responseTime):
			}
		}
	}

	output(attempts, successfulProbes, ring.values(), displayHost, port)
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

func output(attempts, successfulProbes int, samples []float64, host string, port int) {
	probesSent := attempts
	if successfulProbes == 0 {
		fmt.Printf("\nAll the requests have failed. The host %s is not replying to connections on %d\n", host, port)
		os.Exit(exitAllFailed)
	}
	percentFailed := 100 - (float64(successfulProbes)*100)/float64(probesSent)

	// Statistics below are computed over the retained sample window. For
	// finite runs this is all probes; for very long infinite runs it is the
	// most recent maxSamples probes.
	sum := 0.0
	smallest := samples[0]
	biggest := samples[0]
	for _, v := range samples {
		sum += v
		if v < smallest {
			smallest = v
		}
		if v > biggest {
			biggest = v
		}
	}
	timeAverage := time.Duration(sum / float64(len(samples)))

	median, _ := stats.Median(samples)
	percentile90, _ := stats.Percentile(samples, 90)
	percentile75, _ := stats.Percentile(samples, 75)
	percentile50, _ := stats.Percentile(samples, 50)
	percentile25, _ := stats.Percentile(samples, 25)

	fmt.Println("\nProbes sent:", probesSent, "\nSuccessful responses:", successfulProbes,
		"\n% of requests failed:", percentFailed,
		"\nMin response time:", time.Duration(smallest),
		"\nAverage response time:", timeAverage,
		"\nMedian response time:", time.Duration(median),
		"\nMax response time:", time.Duration(biggest))

	fmt.Println("\n90% of requests were faster than:", time.Duration(percentile90),
		"\n75% of requests were faster than:", time.Duration(percentile75),
		"\n50% of requests were faster than:", time.Duration(percentile50),
		"\n25% of requests were faster than:", time.Duration(percentile25))
}

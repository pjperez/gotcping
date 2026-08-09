package main

import (
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// probeResult is the outcome of a single probe.
type probeResult struct {
	number    int
	ok        bool
	canceled  bool // probe aborted by stop/deadline; not recorded as failure
	rtt       time.Duration
	tlsRTT    time.Duration
	kernelRTT time.Duration
	err       error
	ts        time.Time
}

// probeEvent is one line of -jsonl output.
type probeEvent struct {
	Event       string  `json:"event"`
	Number      int     `json:"number"`
	OK          bool    `json:"ok"`
	RTTMs       float64 `json:"rtt_ms,omitempty"`
	TLSMs       float64 `json:"tls_ms,omitempty"`
	KernelRTTMs float64 `json:"kernel_rtt_ms,omitempty"`
	Error       string  `json:"error,omitempty"`
	Timestamp   string  `json:"ts"`
}

// ping runs the probe loop for a single target and returns the process exit code.
func ping(displayHost, resolvedIP string, port int, opts options) int {
	addr := net.JoinHostPort(resolvedIP, strconv.Itoa(port))

	dialer, err := buildDialer(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return exitUsage
	}

	// Always allow Ctrl+C / SIGTERM (or a deadline) to stop and still print
	// results, for both finite and infinite runs.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	var deadlineCh <-chan time.Time
	if opts.deadline > 0 {
		deadlineCh = time.After(time.Duration(opts.deadline) * time.Second)
	}

	// One context drives every dial, TLS handshake, and the pacing loop, so a
	// stop/deadline aborts in-flight probes promptly instead of waiting out
	// each dial timeout.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-stop:
			cancel()
		case <-deadlineCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	var cw *csv.Writer
	if opts.csv {
		cw = csv.NewWriter(os.Stdout)
		if opts.csvHeader {
			if err := cw.Write(csvProbeHeader); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				return exitUsage
			}
		}
	}

	successful := 0
	ring := newSampleRing(maxSamples)
	var kernelSamples []float64

	record := func(pr probeResult) {
		if pr.canceled {
			return
		}
		if pr.ok {
			successful++
			ring.add(float64(pr.rtt))
			if pr.kernelRTT > 0 && len(kernelSamples) < maxSamples {
				kernelSamples = append(kernelSamples, float64(pr.kernelRTT))
			}
		}
		emitProbe(pr, opts, displayHost, port, cw)
	}

	attempts := runProbeLoop(ctx, addr, dialer, opts, record)

	code := emitSummary(attempts, successful, ring.values(), kernelSamples, displayHost, port, opts, cw)
	if cw != nil {
		cw.Flush()
		if err := cw.Error(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
	}
	return code
}

// runProbeLoop paces probe launches at opts.interval and runs up to
// opts.concurrency in flight. The generator is the only writer of attempt
// numbers; the collector is the only writer of counters/ring, so no locking is
// needed between them. Returns the number of probes launched.
func runProbeLoop(ctx context.Context, addr string, dialer *net.Dialer, opts options, record func(probeResult)) int {
	work := make(chan int, opts.concurrency)
	results := make(chan probeResult, opts.concurrency)

	var wg sync.WaitGroup
	for i := 0; i < opts.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range work {
				results <- runOneProbe(ctx, n, addr, dialer, opts)
			}
		}()
	}

	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		for pr := range results {
			record(pr)
		}
	}()

	launched := 0
outer:
	for {
		if opts.count >= 1 && launched >= opts.count {
			break
		}
		select {
		case <-ctx.Done():
			break outer
		default:
		}
		launched++
		select {
		case work <- launched:
		case <-ctx.Done():
			launched-- // probe never handed off; don't count it
			break outer
		}
		if opts.count < 1 || launched < opts.count {
			select {
			case <-time.After(opts.interval):
			case <-ctx.Done():
				break outer
			}
		}
	}
	close(work)
	wg.Wait()
	close(results)
	<-collectorDone
	return launched
}

// runOneProbe performs a single TCP (optionally TLS) probe.
func runOneProbe(ctx context.Context, n int, addr string, d *net.Dialer, opts options) probeResult {
	res := probeResult{number: n}
	start := time.Now()
	conn, err := dialAddr(ctx, d, opts, addr)
	if err != nil {
		if ctx.Err() != nil {
			res.canceled = true
			return res
		}
		res.err = err
		return res
	}
	res.rtt = time.Since(start)

	if opts.kernelRTT {
		if kr, ok := kernelRTT(conn); ok {
			res.kernelRTT = kr
		}
	}

	if opts.sni != "" {
		defer func() { _ = conn.Close() }()
		if err := conn.SetDeadline(time.Now().Add(opts.timeout)); err != nil {
			res.err = err
			return res
		}
		tconn := tls.Client(conn, &tls.Config{
			ServerName:         opts.sni,
			InsecureSkipVerify: opts.insecureTLS,
		})
		tstart := time.Now()
		if err := tconn.HandshakeContext(ctx); err != nil {
			if ctx.Err() != nil {
				res.canceled = true
				return res
			}
			res.err = fmt.Errorf("TLS handshake: %w", err)
			return res
		}
		res.tlsRTT = time.Since(tstart)
	} else {
		_ = conn.Close()
	}
	res.ok = true
	return res
}

// dialAddr dials addr, binding a random source port from opts' range when one
// is configured (a shared fixed source port would collide under concurrency).
func dialAddr(ctx context.Context, d *net.Dialer, opts options, addr string) (net.Conn, error) {
	if opts.sourcePortStart == 0 {
		return d.DialContext(ctx, "tcp", addr)
	}
	d2 := *d
	tcp := new(net.TCPAddr)
	if ta, ok := d.LocalAddr.(*net.TCPAddr); ok {
		*tcp = *ta
	}
	tcp.Port = opts.sourcePortStart + rand.IntN(opts.sourcePortEnd-opts.sourcePortStart+1)
	d2.LocalAddr = tcp
	return d2.DialContext(ctx, "tcp", addr)
}

// buildDialer builds the shared dialer, binding a source IP/interface if one
// was requested.
func buildDialer(opts options) (*net.Dialer, error) {
	d := &net.Dialer{Timeout: opts.timeout}
	if opts.source == "" {
		return d, nil
	}
	ip, err := parseSource(opts.source)
	if err != nil {
		return nil, err
	}
	d.LocalAddr = &net.TCPAddr{IP: ip}
	return d, nil
}

// emitProbe renders a single probe result in the active output mode.
func emitProbe(pr probeResult, opts options, host string, port int, cw *csv.Writer) {
	ts := ""
	if opts.timestamps {
		pr.ts = time.Now()
		ts = pr.ts.Format("[15:04:05.000] ")
	}
	switch {
	case opts.jsonl:
		ev := probeEvent{
			Event:     "probe",
			Number:    pr.number,
			OK:        pr.ok,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		}
		if pr.ok {
			ev.RTTMs = ms(pr.rtt)
			if pr.tlsRTT > 0 {
				ev.TLSMs = ms(pr.tlsRTT)
			}
			if pr.kernelRTT > 0 {
				ev.KernelRTTMs = ms(pr.kernelRTT)
			}
		} else if pr.err != nil {
			ev.Error = pr.err.Error()
		}
		b, err := json.Marshal(ev)
		if err == nil {
			fmt.Println(string(b))
		}
	case opts.csv:
		if cw != nil {
			_ = cw.Write(probeCSVRow(pr))
		}
	default:
		if !pr.ok {
			fmt.Fprintf(os.Stderr, "%sFailed to connect to %s on port %d: %v\n", ts, host, port, pr.err)
			return
		}
		if opts.quiet || opts.json {
			return
		}
		fmt.Printf("%sProbe %d: Connected to %s:%d, RTT=%.2fms", ts, pr.number, host, port, ms(pr.rtt))
		if pr.tlsRTT > 0 {
			fmt.Printf(", TLS handshake=%.2fms", ms(pr.tlsRTT))
		}
		if pr.kernelRTT > 0 {
			fmt.Printf(", kernel RTT=%.2fms", ms(pr.kernelRTT))
		}
		fmt.Println()
	}
}

// probeCSVRow renders a probe result as a CSV record.
func probeCSVRow(pr probeResult) []string {
	ok, rtt := "false", ""
	if pr.ok {
		ok, rtt = "true", strconv.FormatFloat(ms(pr.rtt), 'f', 3, 64)
	}
	tlsMs, kr := "", ""
	if pr.tlsRTT > 0 {
		tlsMs = strconv.FormatFloat(ms(pr.tlsRTT), 'f', 3, 64)
	}
	if pr.kernelRTT > 0 {
		kr = strconv.FormatFloat(ms(pr.kernelRTT), 'f', 3, 64)
	}
	errStr := ""
	if pr.err != nil {
		errStr = pr.err.Error()
	}
	return []string{"probe", strconv.Itoa(pr.number), ok, rtt, tlsMs, kr, errStr}
}

// ms converts a duration to milliseconds as a float64.
func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// sampleRing is a fixed-capacity ring buffer of float64 samples. Once full,
// new samples overwrite the oldest. values() returns the current contents in
// insertion order. It is written by a single goroutine (the collector).
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

package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Exit codes (kept stable for scripts/CI wrapping this tool).
const (
	exitOK            = 0
	exitUsage         = 1
	exitResolveFailed = 2
	exitAllFailed     = 3
	exitLossExceeded  = 4
)

// Version is the current version of gotcping. It defaults to "dev" for
// source builds and is overridden at release time via ldflags, e.g.
//
//	go build -ldflags="-X main.Version=0.7.0" .
var Version = "dev"

// maxSamples caps how many RTT samples are retained for statistics. In
// infinite mode this bounds memory; for typical finite runs it is never hit.
const maxSamples = 100000

func main() {
	hostPtr := flag.String("host", "", "Host or IP address to test")
	portPtr := flag.Int("port", 80, "Port number to query (1-65535)")
	countPtr := flag.Int("count", 10, "Number of requests to send [0 or negative means infinite]")
	timeoutPtr := flag.Float64("timeout", 1.0, "Per-probe dial timeout, in seconds (>= 0.001)")
	deadlinePtr := flag.Int("deadline", 0, "Stop after this many seconds regardless of count [0 means no deadline]")
	intervalPtr := flag.Float64("i", 1.0, "Seconds between probes (>= 0.001)")
	concurrencyPtr := flag.Int("P", 1, "Number of concurrent in-flight probes (>= 1)")
	quietPtr := flag.Bool("q", false, "Quiet: suppress per-probe lines, print only the summary")
	jsonPtr := flag.Bool("json", false, "Emit results as a single JSON object on stdout")
	jsonlPtr := flag.Bool("jsonl", false, "Emit one JSON object per probe plus a summary line")
	csvPtr := flag.Bool("csv", false, "Emit probe and summary rows as CSV on stdout")
	histPtr := flag.Bool("hist", false, "Print an ASCII RTT histogram with the text summary")
	tlsPtr := flag.Bool("tls", false, "Also measure the TLS handshake (SNI defaults to the host)")
	sniPtr := flag.String("sni", "", "SNI/ServerName for the TLS handshake (implies TLS probing)")
	insecurePtr := flag.Bool("insecure", false, "Skip TLS certificate verification")
	kernelRttPtr := flag.Bool("rtt", false, "Report the kernel-smoothed RTT via TCP_INFO (Linux)")
	sourcePtr := flag.String("source", "", "Bind probes to this source IP or interface name")
	sourcePortPtr := flag.String("source-port", "", "Bind source ports in a range, e.g. 2000-3000")
	tsPtr := flag.Bool("t", false, "Prefix per-probe lines with a millisecond timestamp")
	exitLossPtr := flag.Float64("exit-loss", -1, "Exit 4 when the %% of failed probes exceeds this (0-100; -1 disables)")
	filePtr := flag.String("file", "", "Read targets (host[:port] per line, '#' comments) and probe them all")
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
	deadline := *deadlinePtr

	timeout, err := validateTimeout(*timeoutPtr)
	if err != nil {
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
	if err := validatePort(port); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	if err := validateDeadline(deadline); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	if *concurrencyPtr < 1 {
		fmt.Fprintf(os.Stderr, "error: concurrency (-P) must be >= 1 (got %d)\n", *concurrencyPtr)
		os.Exit(exitUsage)
	}
	if *exitLossPtr < -1 || *exitLossPtr > 100 {
		fmt.Fprintf(os.Stderr, "error: -exit-loss must be between 0 and 100 (got %g)\n", *exitLossPtr)
		os.Exit(exitUsage)
	}
	sourceStart, sourceEnd, err := parsePortRange(*sourcePortPtr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitUsage)
	}
	if *sourcePtr != "" {
		if _, err := parseSource(*sourcePtr); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(exitUsage)
		}
	}

	opts := options{
		count:           count,
		timeout:         timeout,
		deadline:        deadline,
		interval:        interval,
		concurrency:     *concurrencyPtr,
		source:          *sourcePtr,
		sourcePortStart: sourceStart,
		sourcePortEnd:   sourceEnd,
		sni:             *sniPtr,
		tls:             *tlsPtr,
		insecureTLS:     *insecurePtr,
		kernelRTT:       *kernelRttPtr,
		exitLoss:        *exitLossPtr,
		quiet:           *quietPtr,
		json:            *jsonPtr,
		jsonl:           *jsonlPtr,
		csv:             *csvPtr,
		histogram:       *histPtr,
		timestamps:      *tsPtr,
		csvHeader:       true,
	}

	if *filePtr != "" {
		os.Exit(runFileMode(*filePtr, port, family, opts))
	}

	if host == "" {
		flag.Usage()
		os.Exit(exitUsage)
	}

	os.Exit(pingTarget(host, port, family, opts))
}

// pingTarget resolves host (once) and runs a ping for it, returning the exit
// code. With -tls and no explicit -sni, the host is used as the SNI name.
func pingTarget(host string, port int, family string, opts options) int {
	resolvedIP, err := resolveHost(host, family)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitResolveFailed
	}
	if opts.tls && opts.sni == "" {
		opts.sni = host
	}
	return ping(sanitize(host), resolvedIP, port, opts)
}

// runFileMode probes every target in path (see readTargets) and returns the
// most severe exit code across all targets.
func runFileMode(path string, defaultPort int, family string, opts options) int {
	targets, err := readTargets(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return exitUsage
	}
	if len(targets) == 0 {
		fmt.Fprintf(os.Stderr, "error: no targets found in %s\n", path)
		return exitUsage
	}

	// Emit the CSV probe header once; ping() must not repeat it per target.
	if opts.csv {
		cw := csv.NewWriter(os.Stdout)
		_ = cw.Write(csvProbeHeader)
		cw.Flush()
		opts.csvHeader = false
	}

	code := exitOK
	for _, t := range targets {
		port := t.port
		if port == 0 {
			port = defaultPort
		}
		if !opts.json && !opts.jsonl && !opts.csv {
			fmt.Printf("== %s:%d ==\n", sanitize(t.host), port)
		}
		if c := pingTarget(t.host, port, family, opts); c > code {
			code = c
		}
	}
	return code
}

// validatePort returns an error if p is outside the valid TCP port range.
func validatePort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port must be between 1 and 65535 (got %d)", p)
	}
	return nil
}

// validateTimeout enforces a positive dial timeout (sub-second allowed, down
// to 1ms) and returns it as a Duration. A non-positive timeout disables the
// dial timeout on most platforms, defeating the tool's own DoS protection and
// hanging the process.
func validateTimeout(seconds float64) (time.Duration, error) {
	if seconds < 0.001 {
		return 0, fmt.Errorf("timeout must be >= 0.001 seconds (got %g)", seconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
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
	count           int
	timeout         time.Duration
	deadline        int
	interval        time.Duration
	concurrency     int
	source          string
	sourcePortStart int
	sourcePortEnd   int
	sni             string
	tls             bool
	insecureTLS     bool
	kernelRTT       bool
	exitLoss        float64
	quiet           bool
	json            bool
	jsonl           bool
	csv             bool
	histogram       bool
	timestamps      bool
	csvHeader       bool // internal: emit the CSV probe header (once in -file mode)
}

// parseSource resolves a source-bind value: either a literal IP address or an
// interface name (its first IPv4 address is used).
func parseSource(s string) (net.IP, error) {
	if ip := net.ParseIP(s); ip != nil {
		return ip, nil
	}
	iface, err := net.InterfaceByName(s)
	if err != nil {
		return nil, fmt.Errorf("can't parse %q as an IP address or interface name", s)
	}
	addrs, err := iface.Addrs()
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("interface %q has no usable addresses", s)
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && ip.To4() != nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("interface %q has no IPv4 address", s)
}

// parsePortRange parses "start" or "start-end" (inclusive) into a port range.
// An empty string yields (0,0) meaning "use ephemeral ports".
func parsePortRange(s string) (int, int, error) {
	if s == "" {
		return 0, 0, nil
	}
	start, end, err := 0, 0, error(nil)
	if lo, hi, ok := strings.Cut(s, "-"); ok {
		start, err = strconv.Atoi(lo)
		if err == nil {
			end, err = strconv.Atoi(hi)
		}
	} else {
		start, err = strconv.Atoi(s)
		end = start
	}
	if err != nil {
		return 0, 0, fmt.Errorf("invalid source-port range %q", s)
	}
	if start < 1 || end > 65535 || start > end {
		return 0, 0, fmt.Errorf("source-port range must satisfy 1 <= start <= end <= 65535 (got %d-%d)", start, end)
	}
	return start, end, nil
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

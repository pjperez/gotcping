package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
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

func main() {
	hostPtr := flag.String("host", "", "Host or IP address to test")
	portPtr := flag.Int("port", 80, "Port number to query (1-65535)")
	countPtr := flag.Int("count", 10, "Number of requests to send [0 means infinite]")
	timeoutPtr := flag.Int("timeout", 1, "Timeout for each request, in seconds (>=1)")

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

	if host == "" {
		flag.Usage()
		os.Exit(exitUsage)
	}
	// H1: validate port range before it reaches net.Dial.
	if port < 1 || port > 65535 {
		fmt.Fprintf(os.Stderr, "error: port must be between 1 and 65535 (got %d)\n", port)
		os.Exit(exitUsage)
	}
	// H2: a non-positive timeout disables the dial timeout entirely on most
	// platforms, so enforce a sane lower bound.
	if timeout < 1 {
		fmt.Fprintf(os.Stderr, "error: timeout must be at least 1 second (got %d)\n", timeout)
		os.Exit(exitUsage)
	}

	// H3: resolve once and dial the resulting IP, so the host we validate is
	// the host we actually connect to (avoids DNS-rebinding TOCTOU between a
	// validation lookup and the dial, and re-resolution per probe).
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		fmt.Printf("error: can't resolve %s\n", host)
		os.Exit(exitResolveFailed)
	}
	resolvedIP := ips[0].String()

	ping(host, resolvedIP, port, count, timeout)
}

func ping(displayHost, resolvedIP string, port int, count int, timeout int) {
	successfulProbes := 0
	i := 1
	timeTotal := time.Duration(0)
	var responseTimes []float64

	// net.JoinHostPort correctly brackets IPv6 literals (also clears the
	// `go vet` "address format does not work with IPv6" warning).
	addr := net.JoinHostPort(resolvedIP, strconv.Itoa(port))

	// In infinite mode (count < 1), allow Ctrl+C / SIGTERM to stop and still
	// print results.
	stop := make(chan os.Signal, 1)
	if count < 1 {
		signal.Notify(stop, os.Interrupt)
	}
	defer signal.Stop(stop)

	for i = 1; (count >= i || count < 1); i++ {
		if count < 1 {
			select {
			case <-stop:
				// i is the probe that was about to start; report i-1 sent.
				output(successfulProbes, timeTotal, displayHost, port, responseTimes, i)
				os.Exit(exitOK)
			default:
			}
		}

		timeStart := time.Now()
		_, err := net.DialTimeout("tcp", addr, time.Second*time.Duration(timeout))
		responseTime := time.Since(timeStart)
		if err != nil {
			fmt.Printf("Received timeout while connecting to %s on port %d.\n", displayHost, port)
		} else {
			fmt.Printf("Probe %v: Connected to %s:%d, RTT=%.2fms\n", i, displayHost, port, float64(responseTime.Nanoseconds())/1e6)
			timeTotal += responseTime
			successfulProbes++
			responseTimes = append(responseTimes, float64(responseTime))
		}

		// Don't sleep after the last needed ping, so results can be displayed 1 second faster
		// (quick mathematics are cheap, 1 second is long)
		if (count-i) > 0 || count <= 0 {
			if count < 1 {
				// Interruptible sleep so Ctrl+C is responsive.
				select {
				case <-stop:
					// Last completed probe is i; report i probes sent.
					output(successfulProbes, timeTotal, displayHost, port, responseTimes, i+1)
					os.Exit(exitOK)
				case <-time.After(time.Second - responseTime):
				}
			} else {
				time.Sleep(time.Second - responseTime)
			}
		}
	}

	// Print results
	output(successfulProbes, timeTotal, displayHost, port, responseTimes, i)
}

func output(successfulProbes int, timeTotal time.Duration, host string, port int, responseTimes []float64, i int) {
	// Let's calculate and spill some results
	// 1. Average response time
	timeAverage := time.Duration(0)
	if successfulProbes > 0 {
		timeAverage = time.Duration(int64(timeTotal) / int64(successfulProbes))
	} else {
		fmt.Printf("\nAll the requests have failed. The host %s is not replying to connections on %d\n", host, port)
		os.Exit(exitAllFailed)
	}
	// 2. Min and Max response times
	var biggest float64

	smallest := float64(1000000000)

	for _, v := range responseTimes {

		if v > biggest {
			biggest = v
		}

		if v < smallest {
			smallest = v
		}

	}

	// 3. Median response time
	median, _ := stats.Median(responseTimes)

	// 4. Percentile
	percentile90, _ := stats.Percentile(responseTimes, float64(90))
	percentile75, _ := stats.Percentile(responseTimes, float64(75))
	percentile50, _ := stats.Percentile(responseTimes, float64(50))
	percentile25, _ := stats.Percentile(responseTimes, float64(25))

	probesSent := i - 1
	percentFailed := 100 - (float64(successfulProbes)*100)/float64(probesSent)

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

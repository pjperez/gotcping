package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// summaryEvent wraps the statistics for -jsonl output.
type summaryEvent struct {
	Event string `json:"event"`
	*statistics
}

var csvProbeHeader = []string{
	"event", "probe", "ok", "rtt_ms", "tls_ms", "kernel_rtt_ms", "error",
}

var csvSummaryHeader = []string{
	"event", "host", "port", "probes_sent", "successful", "failed", "percent_failed",
	"min_ms", "avg_ms", "median_ms", "max_ms", "stddev_ms", "jitter_ms",
	"p25_ms", "p50_ms", "p75_ms", "p90_ms", "p95_ms", "p99_ms", "kernel_rtt_avg_ms",
}

// emitSummary computes and renders the run summary, returning the process exit
// code (all-failed or loss-threshold-exceeded).
func emitSummary(attempts, successful int, samples, kernelSamples []float64, host string, port int, opts options, cw *csv.Writer) int {
	s, err := computeStats(attempts, successful, samples, kernelSamples, host, port)
	if err != nil {
		// All probes failed.
		if opts.json {
			b, _ := json.Marshal(struct {
				Error string `json:"error"`
				Host  string `json:"host"`
				Port  int    `json:"port"`
			}{sanitize(err.Error()), sanitize(host), port})
			fmt.Fprintln(os.Stderr, string(b))
		} else {
			fmt.Fprintf(os.Stderr, "\nAll the requests have failed. The host %s is not replying to connections on %d\n", sanitize(host), port)
		}
		return exitAllFailed
	}
	switch {
	case opts.json:
		b, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return exitOK
		}
		fmt.Println(string(b))
	case opts.jsonl:
		b, err := json.Marshal(summaryEvent{Event: "summary", statistics: s})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return exitOK
		}
		fmt.Println(string(b))
	case opts.csv:
		if cw != nil {
			_ = cw.Write([]string{})
			_ = cw.Write(csvSummaryHeader)
			_ = cw.Write(summaryCSVRow(s))
		}
	default:
		printTextSummary(s)
		if opts.histogram {
			ms := make([]float64, len(samples))
			for i, v := range samples {
				ms[i] = v / 1e6
			}
			fmt.Print(renderHistogram(computeHistogram(ms, 20)))
		}
	}
	if opts.exitLoss >= 0 && s.PercentFailed > opts.exitLoss {
		return exitLossExceeded
	}
	return exitOK
}

// summaryCSVRow renders the summary as one CSV record.
func summaryCSVRow(s *statistics) []string {
	return []string{
		"summary", s.Host, strconv.Itoa(s.Port),
		strconv.Itoa(s.ProbesSent), strconv.Itoa(s.Successful), strconv.Itoa(s.Failed),
		strconv.FormatFloat(s.PercentFailed, 'f', 4, 64),
		strconv.FormatFloat(s.MinMs, 'f', 4, 64),
		strconv.FormatFloat(s.AvgMs, 'f', 4, 64),
		strconv.FormatFloat(s.MedianMs, 'f', 4, 64),
		strconv.FormatFloat(s.MaxMs, 'f', 4, 64),
		strconv.FormatFloat(s.StdDevMs, 'f', 4, 64),
		strconv.FormatFloat(s.JitterMs, 'f', 4, 64),
		strconv.FormatFloat(s.P25Ms, 'f', 4, 64),
		strconv.FormatFloat(s.P50Ms, 'f', 4, 64),
		strconv.FormatFloat(s.P75Ms, 'f', 4, 64),
		strconv.FormatFloat(s.P90Ms, 'f', 4, 64),
		strconv.FormatFloat(s.P95Ms, 'f', 4, 64),
		strconv.FormatFloat(s.P99Ms, 'f', 4, 64),
		strconv.FormatFloat(s.KernelRttAvgMs, 'f', 4, 64),
	}
}

// printTextSummary renders the human-readable summary block.
func printTextSummary(s *statistics) {
	fmt.Println("\nProbes sent:", s.ProbesSent,
		"\nSuccessful responses:", s.Successful,
		"\n% of requests failed:", s.PercentFailed,
		"\nMin response time:", time.Duration(s.MinMs*1e6),
		"\nAverage response time:", time.Duration(s.AvgMs*1e6),
		"\nMedian response time:", time.Duration(s.MedianMs*1e6),
		"\nMax response time:", time.Duration(s.MaxMs*1e6),
		"\nStd deviation:", time.Duration(s.StdDevMs*1e6),
		"\nJitter (mean abs delta):", time.Duration(s.JitterMs*1e6))
	if s.KernelRttAvgMs > 0 {
		fmt.Println("Kernel RTT (avg):", time.Duration(s.KernelRttAvgMs*1e6))
	}

	fmt.Println("\n90% of requests were faster than:", time.Duration(s.P90Ms*1e6),
		"\n75% of requests were faster than:", time.Duration(s.P75Ms*1e6),
		"\n50% of requests were faster than:", time.Duration(s.P50Ms*1e6),
		"\n25% of requests were faster than:", time.Duration(s.P25Ms*1e6),
		"\n95% of requests were faster than:", time.Duration(s.P95Ms*1e6),
		"\n99% of requests were faster than:", time.Duration(s.P99Ms*1e6))
}

// renderHistogram renders an ASCII bar chart of the RTT histogram.
func renderHistogram(bins []histBin) string {
	if len(bins) == 0 {
		return ""
	}
	maxCount := 0
	for _, b := range bins {
		if b.Count > maxCount {
			maxCount = b.Count
		}
	}
	var sb strings.Builder
	sb.WriteString("\nRTT histogram (ms):\n")
	for _, b := range bins {
		barLen := 0
		if maxCount > 0 {
			barLen = int(float64(b.Count) * 40 / float64(maxCount))
		}
		sb.WriteString(fmt.Sprintf("%8.3f - %8.3f | %s (%d)\n",
			b.Lo, b.Hi, strings.Repeat("#", barLen), b.Count))
	}
	return sb.String()
}

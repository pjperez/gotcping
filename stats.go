package main

import (
	"errors"

	"github.com/montanaflynn/stats"
)

// statistics is the computed summary of a run. All latency fields are in
// milliseconds. It is serialized directly for the -json and -jsonl output
// modes.
type statistics struct {
	Host           string  `json:"host"`
	Port           int     `json:"port"`
	ProbesSent     int     `json:"probes_sent"`
	Successful     int     `json:"successful"`
	Failed         int     `json:"failed"`
	PercentFailed  float64 `json:"percent_failed"`
	MinMs          float64 `json:"min_ms"`
	AvgMs          float64 `json:"avg_ms"`
	MedianMs       float64 `json:"median_ms"`
	MaxMs          float64 `json:"max_ms"`
	StdDevMs       float64 `json:"stddev_ms"`
	JitterMs       float64 `json:"jitter_ms"`
	P25Ms          float64 `json:"p25_ms"`
	P50Ms          float64 `json:"p50_ms"`
	P75Ms          float64 `json:"p75_ms"`
	P90Ms          float64 `json:"p90_ms"`
	P95Ms          float64 `json:"p95_ms"`
	P99Ms          float64 `json:"p99_ms"`
	KernelRttAvgMs float64 `json:"kernel_rtt_avg_ms"`
}

// computeStats summarizes a run. samples are RTTs in nanoseconds as float64
// (one entry per successful probe); kernelSamples are TCP_INFO RTTs in
// nanoseconds and may be empty. It returns an error if there were no
// successful probes.
func computeStats(attempts, successful int, samples, kernelSamples []float64, host string, port int) (*statistics, error) {
	if successful == 0 || len(samples) == 0 {
		return nil, errors.New("no successful probes")
	}

	toMs := func(ns float64) float64 { return ns / 1e6 }

	ms := make([]float64, len(samples))
	for i, v := range samples {
		ms[i] = toMs(v)
	}

	sum := 0.0
	minMs := ms[0]
	maxMs := ms[0]
	for _, v := range ms {
		sum += v
		if v < minMs {
			minMs = v
		}
		if v > maxMs {
			maxMs = v
		}
	}
	avg := sum / float64(len(ms))

	median, _ := stats.Median(ms)
	stddev, _ := stats.StandardDeviation(ms)
	p25, _ := stats.Percentile(ms, 25)
	p75, _ := stats.Percentile(ms, 75)
	p90, _ := stats.Percentile(ms, 90)
	p95, _ := stats.Percentile(ms, 95)
	p99, _ := stats.Percentile(ms, 99)

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

	kernelAvg := 0.0
	if len(kernelSamples) > 0 {
		var ksum float64
		for _, v := range kernelSamples {
			ksum += toMs(v)
		}
		kernelAvg = ksum / float64(len(kernelSamples))
	}

	return &statistics{
		Host:           host,
		Port:           port,
		ProbesSent:     attempts,
		Successful:     successful,
		Failed:         failed,
		PercentFailed:  percentFailed,
		MinMs:          minMs,
		AvgMs:          avg,
		MedianMs:       median,
		MaxMs:          maxMs,
		StdDevMs:       stddev,
		JitterMs:       jitter,
		P25Ms:          p25,
		P50Ms:          median,
		P75Ms:          p75,
		P90Ms:          p90,
		P95Ms:          p95,
		P99Ms:          p99,
		KernelRttAvgMs: kernelAvg,
	}, nil
}

// histBin is one bucket of an RTT histogram.
type histBin struct {
	Lo, Hi float64
	Count  int
}

// computeHistogram buckets ms values into a fixed number of equal-width bins.
// An empty input yields nil.
func computeHistogram(ms []float64, bins int) []histBin {
	if len(ms) == 0 {
		return nil
	}
	if bins < 1 {
		bins = 1
	}
	minV, maxV := ms[0], ms[0]
	for _, v := range ms {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	width := (maxV - minV) / float64(bins)
	if width == 0 {
		width = 1 // all samples identical
	}
	res := make([]histBin, bins)
	for i := range res {
		res[i] = histBin{Lo: minV + width*float64(i), Hi: minV + width*float64(i+1)}
	}
	res[bins-1].Hi = maxV
	for _, v := range ms {
		idx := int((v - minV) / width)
		if idx >= bins {
			idx = bins - 1
		}
		res[idx].Count++
	}
	return res
}

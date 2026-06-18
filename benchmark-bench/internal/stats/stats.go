package stats

import (
	"math"
	"math/rand"
	"sort"
)

// Summary aggregates T_first samples.
type Summary struct {
	Mean   float64 `json:"meanNs"`
	P50    float64 `json:"p50Ns"`
	P95    float64 `json:"p95Ns"`
	StdErr float64 `json:"stdErrNs"`
	CV     float64 `json:"cvPercent"`
	CILow  float64 `json:"ci95LowNs"`
	CIHigh float64 `json:"ci95HighNs"`
	N      int     `json:"n"`
}

// Summarize computes statistics for latency samples in nanoseconds.
func Summarize(values []float64) Summary {
	return SummarizeFiltered(values, values)
}

// SummarizeFiltered computes stats on filtered values while reporting N from all samples.
func SummarizeFiltered(all []float64, filtered []float64) Summary {
	n := len(filtered)
	if n == 0 {
		return Summary{N: len(all)}
	}
	cp := append([]float64(nil), filtered...)
	sort.Float64s(cp)
	sum := 0.0
	for _, v := range cp {
		sum += v
	}
	mean := sum / float64(n)
	variance := 0.0
	for _, v := range cp {
		d := v - mean
		variance += d * d
	}
	variance /= float64(n)
	std := math.Sqrt(variance)
	stdErr := std / math.Sqrt(float64(n))
	cv := 0.0
	if mean > 0 {
		cv = (std / mean) * 100
	}
	ciLow, ciHigh := bootstrapCI(cp, 2000, 0.95)
	return Summary{
		Mean:   mean,
		P50:    percentile(cp, 0.50),
		P95:    percentile(cp, 0.95),
		StdErr: stdErr,
		CV:     cv,
		CILow:  ciLow,
		CIHigh: ciHigh,
		N:      len(all),
	}
}

// FilterIQR removes outliers outside 1.5×IQR; returns filtered slice (may equal input).
func FilterIQR(sorted []float64) []float64 {
	if len(sorted) < 4 {
		return sorted
	}
	cp := append([]float64(nil), sorted...)
	sort.Float64s(cp)
	q1 := percentile(cp, 0.25)
	q3 := percentile(cp, 0.75)
	iqr := q3 - q1
	low := q1 - 1.5*iqr
	high := q3 + 1.5*iqr
	out := make([]float64, 0, len(cp))
	for _, v := range cp {
		if v >= low && v <= high {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return cp
	}
	return out
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func bootstrapCI(data []float64, iterations int, level float64) (float64, float64) {
	if len(data) == 0 {
		return 0, 0
	}
	rng := rand.New(rand.NewSource(42))
	means := make([]float64, iterations)
	n := len(data)
	for i := 0; i < iterations; i++ {
		sum := 0.0
		for j := 0; j < n; j++ {
			sum += data[rng.Intn(n)]
		}
		means[i] = sum / float64(n)
	}
	sort.Float64s(means)
	alpha := (1 - level) / 2
	lowIdx := int(math.Floor(float64(iterations) * alpha))
	highIdx := int(math.Ceil(float64(iterations)*(1-alpha))) - 1
	if lowIdx < 0 {
		lowIdx = 0
	}
	if highIdx >= iterations {
		highIdx = iterations - 1
	}
	return means[lowIdx], means[highIdx]
}

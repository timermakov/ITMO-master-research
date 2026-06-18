package stats

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// Summary aggregates T_first samples.
type Summary struct {
	Mean      float64 `json:"meanNs"`
	P50       float64 `json:"p50Ns"`
	P95       float64 `json:"p95Ns"`
	StdErr    float64 `json:"stdErrNs"`
	CV        float64 `json:"cvPercent"`
	CILow     float64 `json:"ci95LowNs"`
	CIHigh    float64 `json:"ci95HighNs"`
	P50CILow  float64 `json:"p50Ci95LowNs"`
	P50CIHigh float64 `json:"p50Ci95HighNs"`
	N         int     `json:"n"`
	NFiltered int     `json:"nFiltered"`
	Outliers  []int   `json:"outlierRunIndexes,omitempty"`
}

// CVVerdict holds pass/fail with a human-readable reason.
type CVVerdict struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason,omitempty"`
}

// Summarize computes statistics for latency samples in nanoseconds.
func Summarize(values []float64) Summary {
	filtered, outliers := FilterIQRIndexed(values)
	return SummarizeFiltered(values, filtered, outliers)
}

// SummarizeFiltered computes stats on filtered values while reporting N from all samples.
func SummarizeFiltered(all []float64, filtered []float64, outlierRuns []int) Summary {
	n := len(filtered)
	if n == 0 {
		return Summary{N: len(all), NFiltered: 0, Outliers: outlierRuns}
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
	ciLow, ciHigh := bootstrapCI(cp, 2000, 0.95, bootstrapMean)
	p50Low, p50High := bootstrapCI(cp, 2000, 0.95, bootstrapPercentile(0.50))
	return Summary{
		Mean:      mean,
		P50:       Percentile(cp, 0.50),
		P95:       Percentile(cp, 0.95),
		StdErr:    stdErr,
		CV:        cv,
		CILow:     ciLow,
		CIHigh:    ciHigh,
		P50CILow:  p50Low,
		P50CIHigh: p50High,
		N:         len(all),
		NFiltered: n,
		Outliers:  outlierRuns,
	}
}

// EvaluateCV checks whether filtered sample CV meets threshold.
func EvaluateCV(sum Summary, threshold float64) CVVerdict {
	if sum.NFiltered < 5 {
		return CVVerdict{
			Pass:   false,
			Reason: fmt.Sprintf("n_filtered=%d < 5", sum.NFiltered),
		}
	}
	if sum.CV <= threshold {
		return CVVerdict{Pass: true}
	}
	return CVVerdict{
		Pass:   false,
		Reason: fmt.Sprintf("cv=%.2f%% > %.2f%%", sum.CV, threshold),
	}
}

// FilterIQR removes outliers outside 1.5×IQR; requires n >= 5.
func FilterIQR(sorted []float64) []float64 {
	filtered, _ := FilterIQRIndexed(sorted)
	return filtered
}

// FilterIQRIndexed returns filtered values and 1-based run indexes marked as outliers.
func FilterIQRIndexed(values []float64) ([]float64, []int) {
	if len(values) < 5 {
		cp := append([]float64(nil), values...)
		return cp, nil
	}
	type indexed struct {
		val float64
		idx int
	}
	items := make([]indexed, len(values))
	for i, v := range values {
		items[i] = indexed{val: v, idx: i + 1}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].val < items[j].val })
	sortedVals := make([]float64, len(items))
	for i, it := range items {
		sortedVals[i] = it.val
	}
	q1 := Percentile(sortedVals, 0.25)
	q3 := Percentile(sortedVals, 0.75)
	iqr := q3 - q1
	low := q1 - 1.5*iqr
	high := q3 + 1.5*iqr
	filtered := make([]float64, 0, len(values))
	outliers := make([]int, 0)
	for _, it := range items {
		if it.val >= low && it.val <= high {
			filtered = append(filtered, it.val)
		} else {
			outliers = append(outliers, it.idx)
		}
	}
	if len(filtered) == 0 {
		cp := append([]float64(nil), values...)
		return cp, nil
	}
	return filtered, outliers
}

// Percentile uses linear interpolation (Hyndman type 7).
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

type bootstrapStat func([]float64) float64

func bootstrapMean(data []float64) float64 {
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func bootstrapPercentile(p float64) bootstrapStat {
	return func(data []float64) float64 {
		cp := append([]float64(nil), data...)
		sort.Float64s(cp)
		return Percentile(cp, p)
	}
}

func bootstrapCI(data []float64, iterations int, level float64, stat bootstrapStat) (float64, float64) {
	if len(data) == 0 {
		return 0, 0
	}
	rng := rand.New(rand.NewSource(42))
	samples := make([]float64, iterations)
	n := len(data)
	for i := 0; i < iterations; i++ {
		batch := make([]float64, n)
		for j := 0; j < n; j++ {
			batch[j] = data[rng.Intn(n)]
		}
		samples[i] = stat(batch)
	}
	sort.Float64s(samples)
	alpha := (1 - level) / 2
	lowIdx := int(math.Floor(float64(iterations) * alpha))
	highIdx := int(math.Ceil(float64(iterations)*(1-alpha))) - 1
	if lowIdx < 0 {
		lowIdx = 0
	}
	if highIdx >= iterations {
		highIdx = iterations - 1
	}
	return samples[lowIdx], samples[highIdx]
}

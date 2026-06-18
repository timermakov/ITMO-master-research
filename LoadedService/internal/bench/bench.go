package bench

import (
	"math"
	"sort"
	"time"

	"github.com/itmo-vkr/loadedservice/internal/mmapstore"
)

// Result carries benchmark numbers for cold and warm random reads.
type Result struct {
	Samples int   `json:"samples"`
	Runs    int   `json:"runs"`
	Seed    int64 `json:"seed"`

	Cold time.Duration `json:"cold"` // mean cold duration across runs
	Warm time.Duration `json:"warm"` // mean warm duration across runs

	ColdMin time.Duration `json:"cold_min"`
	ColdMax time.Duration `json:"cold_max"`
	WarmMin time.Duration `json:"warm_min"`
	WarmMax time.Duration `json:"warm_max"`

	ColdP95 time.Duration `json:"cold_p95"`
	ColdP99 time.Duration `json:"cold_p99"`
	WarmP95 time.Duration `json:"warm_p95"`
	WarmP99 time.Duration `json:"warm_p99"`

	Speedup    float64 `json:"speedup"`     // mean cold/warm ratio across runs
	SpeedupMin float64 `json:"speedup_min"` // min cold/warm across runs
	SpeedupMax float64 `json:"speedup_max"` // max cold/warm across runs
	SpeedupP95 float64 `json:"speedup_p95"`
	SpeedupP99 float64 `json:"speedup_p99"`
}

// RunColdWarm performs a single cold vs warm run using a deterministic page
// access pattern defined by the provided seed.
func RunColdWarm(store *mmapstore.Store, samples int, seed int64) (Result, error) {
	return RunColdWarmMany(store, samples, 1, seed)
}

// RunColdWarmMany performs multiple cold vs warm runs and aggregates statistics.
func RunColdWarmMany(store *mmapstore.Store, samples, runs int, seed int64) (Result, error) {
	if samples <= 0 {
		return Result{}, nil
	}
	if runs <= 0 {
		runs = 1
	}

	coldTotals := make([]time.Duration, runs)
	warmTotals := make([]time.Duration, runs)
	speedups := make([]float64, runs)

	for i := 0; i < runs; i++ {
		runSeed := seed + int64(i)
		indices := store.RandomPageIndices(samples, runSeed)
		if len(indices) == 0 {
			return Result{}, nil
		}

		cold := store.MeasureReadDurationAt(indices)
		if err := store.TouchSequential(); err != nil {
			return Result{}, err
		}
		warm := store.MeasureReadDurationAt(indices)

		coldTotals[i] = cold
		warmTotals[i] = warm
		if warm > 0 {
			speedups[i] = float64(cold) / float64(warm)
		}
	}

	coldMean := meanDuration(coldTotals)
	warmMean := meanDuration(warmTotals)

	res := Result{
		Samples: samples,
		Runs:    runs,
		Seed:    seed,

		Cold:    coldMean,
		Warm:    warmMean,
		ColdMin: minDuration(coldTotals),
		ColdMax: maxDuration(coldTotals),
		WarmMin: minDuration(warmTotals),
		WarmMax: maxDuration(warmTotals),

		ColdP95: percentileDuration(coldTotals, 0.95),
		ColdP99: percentileDuration(coldTotals, 0.99),
		WarmP95: percentileDuration(warmTotals, 0.95),
		WarmP99: percentileDuration(warmTotals, 0.99),

		Speedup:    meanFloat(speedups),
		SpeedupMin: minFloat(speedups),
		SpeedupMax: maxFloat(speedups),
		SpeedupP95: percentileFloat(speedups, 0.95),
		SpeedupP99: percentileFloat(speedups, 0.99),
	}

	return res, nil
}

func sumDurations(values []time.Duration) time.Duration {
	var total time.Duration
	for _, v := range values {
		total += v
	}
	return total
}

func meanDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	return sumDurations(values) / time.Duration(len(values))
}

func minDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func percentileDuration(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	if percentile <= 0 {
		return values[0]
	}
	if percentile >= 1 {
		return values[len(values)-1]
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	} else if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func meanFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func minFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func percentileFloat(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if percentile <= 0 {
		return values[0]
	}
	if percentile >= 1 {
		return values[len(values)-1]
	}
	sorted := append([]float64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	} else if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

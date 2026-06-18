package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/itmo-vkr/dwss/benchmark-bench/internal/cold"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/coord"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/loadgen"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/probe"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/profile"
	"github.com/itmo-vkr/dwss/benchmark-bench/internal/stats"
	"github.com/itmo-vkr/dwss/warmkit"
)

// RunRecord is one independent cold measurement.
type RunRecord struct {
	RunIndex  int           `json:"runIndex"`
	Probe     probe.Sample  `json:"probe"`
	Warmkit   warmkit.State `json:"warmkitState,omitempty"`
	ReadyOK   bool          `json:"readyOk,omitempty"`
	SessionID string        `json:"sessionId,omitempty"`
}

// Result is one scenario output.
type Result struct {
	Scenario   string        `json:"scenario"`
	Summary    stats.Summary `json:"summary"`
	E2ESummary stats.Summary `json:"e2eSummary,omitempty"`
	Values     []float64     `json:"valuesNs"`
	E2EValues  []float64     `json:"e2eValuesNs,omitempty"`
	Runs       []RunRecord   `json:"runs"`
	CVPass     bool          `json:"cvPass"`
	Hypothesis string        `json:"hypothesisNote,omitempty"`
}

// RunS0Control: each run = reset → probe (no warmup).
func RunS0Control(m profile.Manifest) (Result, error) {
	return runIndependent(m, "s0-control", func(ctx context.Context) error {
		return nil
	}, false)
}

// RunSRef: each run = reset → POST /warmup → probe.
func RunSRef(m profile.Manifest) (Result, error) {
	return runIndependent(m, "s-ref", func(ctx context.Context) error {
		body, _ := json.Marshal(m.Profile)
		resp, err := http.Post(m.WarmupURL+"/warmup", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}, false)
}

// RunSDw: each run = reset → baseline mirror → session + loadgen → wait ready → probe.
func RunSDw(ctx context.Context, m profile.Manifest) (Result, error) {
	return runIndependent(m, "s-dw", func(runCtx context.Context) error {
		if err := coord.BaselineMirror(m.CoordURL, m.ActiveInst); err != nil {
			return err
		}
		sid, err := coord.StartSession(m.CoordURL, m.TargetInst, m.ActiveInst, m.ReadyAfter)
		if err != nil {
			return err
		}
		loadCtx, cancel := context.WithTimeout(runCtx, time.Duration(m.LoadSec)*time.Second)
		defer cancel()
		errCh := make(chan error, 1)
		go func() {
			errCh <- loadgen.Run(loadCtx, m.ProxyURL, m.Profile, m.RPS, m.LoadSec)
		}()
		waitErr := coord.WaitSessionCompleted(m.CoordURL, sid, m.LoadSec+30)
		loadErr := <-errCh
		if waitErr != nil {
			return waitErr
		}
		if loadErr != nil && loadErr != context.Canceled && loadErr != context.DeadlineExceeded {
			return loadErr
		}
		return cold.WaitReady(m.WarmupURL, time.Duration(m.LoadSec+30)*time.Second)
	}, true)
}

// RunH4Overhead compares E2E p95 through proxy with mirror off vs on.
func RunH4Overhead(ctx context.Context, m profile.Manifest) ([]Result, error) {
	out := make([]Result, 0, 2)
	phases := []struct {
		name    string
		enabled bool
		ratio   float64
	}{
		{"h4-mirror-off", false, 0},
		{"h4-mirror-on", true, 1.0},
	}
	for _, ph := range phases {
		cfg := warmkit.MirrorConfig{
			Enabled:          ph.enabled,
			Ratio:            ph.ratio,
			TargetInstanceID: m.TargetInst,
			ActiveInstanceID: m.ActiveInst,
		}
		if err := coord.MirrorConfig(m.CoordURL, cfg); err != nil {
			return nil, err
		}
		m.Cooldown()
		loadCtx, cancel := context.WithTimeout(ctx, time.Duration(m.H4LoadSec)*time.Second)
		rtts, err := loadgen.CollectRTT(loadCtx, m.ProxyURL, m.Profile, m.RPS, m.H4LoadSec)
		cancel()
		if err != nil && len(rtts) == 0 {
			return nil, err
		}
		floats := intsToFloats(rtts)
		sorted := append([]float64(nil), floats...)
		sort.Float64s(sorted)
		filtered := stats.FilterIQR(sorted)
		sum := stats.SummarizeFiltered(floats, filtered)
		out = append(out, Result{
			Scenario:   ph.name,
			Summary:    sum,
			Values:     floats,
			CVPass:     sum.CV <= m.CVThreshold,
			Hypothesis: "H4: p95 mirror-on vs mirror-off on proxy /work E2E",
		})
	}
	_ = coord.BaselineMirror(m.CoordURL, m.ActiveInst)
	return out, nil
}

func runIndependent(m profile.Manifest, name string, warmup func(context.Context) error, trackReady bool) (Result, error) {
	records := make([]RunRecord, 0, m.Runs)
	values := make([]float64, 0, m.Runs)
	e2e := make([]float64, 0, m.Runs)
	for i := 0; i < m.Runs; i++ {
		if err := cold.ResetWarmup(m.WarmupURL); err != nil {
			return Result{}, err
		}
		m.Cooldown()
		runCtx := context.Background()
		if err := warmup(runCtx); err != nil {
			return Result{}, err
		}
		s, err := probe.TFirst(m.WarmupURL, m.Profile)
		if err != nil {
			return Result{}, err
		}
		rec := RunRecord{RunIndex: i + 1, Probe: s}
		if trackReady {
			rec.ReadyOK = readyzOK(m.WarmupURL)
		}
		records = append(records, rec)
		values = append(values, float64(s.WorkloadNs))
		e2e = append(e2e, float64(s.E2ENs))
		m.Cooldown()
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	filtered := stats.FilterIQR(sorted)
	sum := stats.SummarizeFiltered(values, filtered)
	e2eSum := stats.Summarize(e2e)
	return Result{
		Scenario:   name,
		Summary:    sum,
		E2ESummary: e2eSum,
		Values:     values,
		E2EValues:  e2e,
		Runs:       records,
		CVPass:     sum.CV <= m.CVThreshold,
	}, nil
}

func readyzOK(warmupURL string) bool {
	resp, err := http.Get(warmupURL + "/readyz")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func intsToFloats(in []int64) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

// EvaluateH4 compares mirror-on vs mirror-off p95 E2E on proxy /work.
func EvaluateH4(off, on Result) string {
	if on.Summary.P95 <= off.Summary.P95*1.05 {
		return "pass"
	}
	return "fail"
}

// EvaluateHypotheses compares scenario medians for H1/H2 notes.
func EvaluateHypotheses(s0, sRef, sDw Result) map[string]string {
	notes := map[string]string{}
	if sDw.Summary.P50 <= s0.Summary.P50/2 {
		notes["H1"] = "pass"
	} else {
		notes["H1"] = "fail"
	}
	if sDw.Summary.P50 <= sRef.Summary.P50*1.1 {
		notes["H2"] = "pass"
	} else {
		notes["H2"] = "fail"
	}
	return notes
}

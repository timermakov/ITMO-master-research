package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	RunIndex    int           `json:"runIndex"`
	Attempt     int           `json:"attempt,omitempty"`
	Probe       probe.Sample  `json:"probe"`
	State       StateSnapshot `json:"state,omitempty"`
	Warmkit     warmkit.State `json:"warmkitState,omitempty"`
	ReadyOK     bool          `json:"readyOk,omitempty"`
	SessionID   string        `json:"sessionId,omitempty"`
	Invalidated bool          `json:"invalidated,omitempty"`
}

// Result is one scenario output.
type Result struct {
	Scenario     string        `json:"scenario"`
	Summary      stats.Summary `json:"summary"`
	E2ESummary   stats.Summary `json:"e2eSummary,omitempty"`
	BlockSummary stats.Summary `json:"blockSummary,omitempty"`
	Values       []float64     `json:"valuesNs"`
	E2EValues    []float64     `json:"e2eValuesNs,omitempty"`
	BlockValues  []float64     `json:"blockValuesNs,omitempty"`
	Runs         []RunRecord   `json:"runs"`
	CVPass       bool          `json:"cvPass"`
	CVPassReason string        `json:"cvPassReason,omitempty"`
	Hypothesis   string        `json:"hypothesisNote,omitempty"`
}

// RunS0Control: each run = reset → probe (no warmup).
func RunS0Control(m profile.Manifest) (Result, error) {
	return runIndependent(m, "s0-control", func(ctx context.Context) error {
		return nil
	}, false, "")
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
		return WaitWarmWorkload(m.WarmupURL, 2*time.Second)
	}, false, "")
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
		lp := m.LoadProfile()
		totalSec := m.TotalLoadSec()
		loadCtx, cancel := context.WithTimeout(runCtx, time.Duration(totalSec+30)*time.Second)
		defer cancel()
		errCh := make(chan error, 1)
		go func() {
			errCh <- loadgen.RunPhased(loadCtx, m.ProxyURL, m.Profile, lp)
		}()
		waitErr := coord.WaitSessionCompleted(m.CoordURL, sid, totalSec+60)
		loadErr := <-errCh
		if waitErr != nil {
			return waitErr
		}
		if loadErr != nil && loadErr != context.Canceled && loadErr != context.DeadlineExceeded {
			return loadErr
		}
		if err := cold.WaitReady(m.WarmupURL, 120*time.Second); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		return nil
	}, true, "")
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
		lp := m.H4LoadProfile()
		totalSec := m.TotalH4LoadSec()
		loadCtx, cancel := context.WithTimeout(ctx, time.Duration(totalSec+30)*time.Second)
		rttRes, err := loadgen.CollectRTTPhased(loadCtx, m.ProxyURL, m.Profile, lp)
		cancel()
		if err != nil && len(rttRes.Steady) == 0 {
			return nil, err
		}
		rawFloats := intsToFloats(rttRes.All)
		blocks := loadgen.BlockMedians(rttRes.Steady, m.RPS)
		filtered, outliers := stats.FilterIQRIndexed(blocks)
		blockSum := stats.SummarizeFiltered(blocks, filtered, outliers)
		cv := stats.EvaluateCV(blockSum, m.CVThreshold)
		out = append(out, Result{
			Scenario:     ph.name,
			Summary:      blockSum,
			BlockSummary: blockSum,
			Values:       rawFloats,
			BlockValues:  blocks,
			CVPass:       cv.Pass,
			CVPassReason: cv.Reason,
			Hypothesis:   "H4: p95 mirror-on vs mirror-off on proxy /work E2E (block medians)",
		})
	}
	_ = coord.BaselineMirror(m.CoordURL, m.ActiveInst)
	return out, nil
}

func runIndependent(m profile.Manifest, name string, warmup func(context.Context) error, trackReady bool, _ string) (Result, error) {
	records := make([]RunRecord, 0, m.MaxRuns)
	values := make([]float64, 0, m.MaxRuns)
	e2e := make([]float64, 0, m.MaxRuns)
	maxValid := m.Runs
	if m.ProfileOnHighCV {
		maxValid = m.MaxRuns
	}
	maxAttempts := m.MaxAttempts()

	for attempt := 1; attempt <= maxAttempts && len(records) < maxValid; attempt++ {
		if err := cold.ResetWarmup(m.WarmupURL); err != nil {
			return Result{}, err
		}
		time.Sleep(200 * time.Millisecond)
		m.Cooldown()
		runCtx := context.Background()
		if err := warmup(runCtx); err != nil {
			return Result{}, err
		}
		readyOK := false
		if trackReady {
			readyOK = readyzOK(m.WarmupURL)
		}
		snap, err := FetchState(m.WarmupURL)
		if err != nil {
			log.Printf("[%s] attempt %d: state fetch failed: %v", name, attempt, err)
			continue
		}
		if !ValidPreProbe(name, snap, readyOK) {
			log.Printf("[%s] attempt %d: invalid pre-probe state (readyOk=%v mmapCold=%v appCold=%v)",
				name, attempt, readyOK, snap.Workload.MmapCold, snap.Workload.AppCold)
			continue
		}
		s, err := probe.TFirst(m.WarmupURL, m.Profile)
		if err != nil {
			log.Printf("[%s] attempt %d: probe failed: %v", name, attempt, err)
			continue
		}
		rec := RunRecord{
			RunIndex: len(records) + 1,
			Attempt:  attempt,
			Probe:    s,
			State:    snap,
			ReadyOK:  readyOK,
		}
		if snap.Warmkit.State != "" {
			rec.Warmkit = snap.Warmkit.State
		}
		records = append(records, rec)
		values = append(values, float64(s.WorkloadNs))
		e2e = append(e2e, float64(s.E2ENs))
		m.Cooldown()

		if len(records) >= m.Runs {
			res := buildResult(name, records, values, e2e, m.CVThreshold)
			if res.CVPass || len(records) >= maxValid || !m.ProfileOnHighCV {
				return res, nil
			}
			log.Printf("[%s] CV fail (%s), collecting up to %d runs", name, res.CVPassReason, maxValid)
		}
	}
	if len(records) == 0 {
		return Result{}, fmt.Errorf("[%s] no valid runs after %d attempts", name, maxAttempts)
	}
	res := buildResult(name, records, values, e2e, m.CVThreshold)
	return res, nil
}

func buildResult(name string, records []RunRecord, values, e2e []float64, cvThreshold float64) Result {
	filtered, outliers := stats.FilterIQRIndexed(values)
	sum := stats.SummarizeFiltered(values, filtered, outliers)
	e2eFiltered, e2eOutliers := stats.FilterIQRIndexed(e2e)
	e2eSum := stats.SummarizeFiltered(e2e, e2eFiltered, e2eOutliers)
	cv := stats.EvaluateCV(sum, cvThreshold)
	return Result{
		Scenario:     name,
		Summary:      sum,
		E2ESummary:   e2eSum,
		Values:       values,
		E2EValues:    e2e,
		Runs:         records,
		CVPass:       cv.Pass,
		CVPassReason: cv.Reason,
	}
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

// EvaluateH4 compares mirror-on vs mirror-off p95 E2E on proxy /work (block medians).
func EvaluateH4(off, on Result) string {
	if on.Summary.P95 <= off.Summary.P95*1.05 {
		return "pass"
	}
	return "fail"
}

// EvaluateHypotheses compares scenario medians for H1/H2/H3 notes.
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
	refBound := sRef.Summary.P50 * 1.1
	if sDw.Summary.P50CIHigh <= refBound {
		notes["H2_CI"] = "pass"
	} else {
		notes["H2_CI"] = "fail"
	}
	h3 := "pass"
	for _, r := range sDw.Runs {
		if !r.ReadyOK {
			h3 = "fail"
			break
		}
	}
	notes["H3"] = h3
	return notes
}

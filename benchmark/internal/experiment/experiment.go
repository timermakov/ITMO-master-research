package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/benchmark/internal/cold"
	"github.com/itmo-vkr/dwss/benchmark/internal/coord"
	"github.com/itmo-vkr/dwss/benchmark/internal/loadgen"
	"github.com/itmo-vkr/dwss/benchmark/internal/probe"
	"github.com/itmo-vkr/dwss/benchmark/internal/profile"
	"github.com/itmo-vkr/dwss/benchmark/internal/progress"
	"github.com/itmo-vkr/dwss/benchmark/internal/stats"
	"github.com/itmo-vkr/dwss/warmkit"
)

// RunRecord is one independent cold measurement.
type RunRecord struct {
	RunIndex           int           `json:"runIndex"`
	Attempt            int           `json:"attempt,omitempty"`
	Probe              probe.Sample  `json:"probe"`
	State              StateSnapshot `json:"state,omitempty"`
	Warmkit            warmkit.State `json:"warmkitState,omitempty"`
	ReadyOK            bool          `json:"readyOk,omitempty"`
	SessionID          string        `json:"sessionId,omitempty"`
	ShadowCountAtProbe int64         `json:"shadowCountAtProbe,omitempty"`
	PostLoadSettleMs   int           `json:"postLoadSettleMs,omitempty"`
	LoadgenDurationMs  int64         `json:"loadgenDurationMs,omitempty"`
	SessionReadyAt     string        `json:"sessionReadyAt,omitempty"`
	Invalidated        bool          `json:"invalidated,omitempty"`
}

// WarmupOutcome carries optional per-run metadata from the warmup phase.
type WarmupOutcome struct {
	SessionID          string
	ShadowCountAtProbe int64
	WarmkitState       string
	PostLoadSettleMs   int
	LoadgenDurationMs  int64
	SessionReadyAt     time.Time
}

// Result is one scenario output.
type Result struct {
	Scenario     string        `json:"scenario"`
	DisplayName  string        `json:"displayName,omitempty"`
	Description  string        `json:"description,omitempty"`
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
	return runIndependent(m, "s0-control", func(context.Context) (WarmupOutcome, error) {
		return WarmupOutcome{}, nil
	}, false)
}

// RunSRef: each run = reset → POST /warmup → probe.
func RunSRef(m profile.Manifest) (Result, error) {
	return runIndependent(m, "s-ref", func(ctx context.Context) (WarmupOutcome, error) {
		body, _ := json.Marshal(m.Profile)
		resp, err := http.Post(m.WarmupURL+"/warmup", "application/json", bytes.NewReader(body))
		if err != nil {
			return WarmupOutcome{}, err
		}
		_ = resp.Body.Close()
		if err := WaitWarmWorkload(m.WarmupURL, 2*time.Second); err != nil {
			return WarmupOutcome{}, err
		}
		return WarmupOutcome{}, nil
	}, false)
}

// RunSDw: each run = reset → session + loadgen (mirror held) → complete → settle → probe.
func RunSDw(ctx context.Context, m profile.Manifest) (Result, error) {
	return runIndependent(m, "s-dw", func(runCtx context.Context) (WarmupOutcome, error) {
		out := WarmupOutcome{PostLoadSettleMs: m.PostLoadSettleMs}
		if err := coord.BaselineMirror(m.CoordURL, m.ActiveInst); err != nil {
			return out, err
		}
		sid, err := coord.StartSession(m.CoordURL, m.TargetInst, m.ActiveInst, m.ReadyAfter)
		if err != nil {
			return out, err
		}
		out.SessionID = sid
		lp := m.LoadProfile()
		totalSec := m.TotalLoadSec()
		log.Printf("[s-dw] session %s started; load profile total=%ds ramp=%ds steady=%ds down=%ds",
			sid, totalSec, lp.RampUpSec, lp.SteadySec, lp.RampDownSec)

		loadCtx, cancel := context.WithTimeout(runCtx, time.Duration(totalSec+30)*time.Second)
		defer cancel()
		loadStart := time.Now()
		errCh := make(chan error, 1)
		go func() {
			errCh <- loadgen.RunPhased(loadCtx, m.ProxyURL, m.Profile, lp)
		}()

		if err := coord.WaitSessionHolding(m.CoordURL, sid, totalSec+60); err != nil {
			return out, err
		}
		out.SessionReadyAt = time.Now().UTC()
		log.Printf("[s-dw] session %s holding mirror at %s", sid, out.SessionReadyAt.Format(time.RFC3339))

		loadErr := <-errCh
		out.LoadgenDurationMs = time.Since(loadStart).Milliseconds()
		if loadErr == nil {
			log.Printf("[s-dw] loadgen completed for session %s in %dms", sid, out.LoadgenDurationMs)
		}
		if loadErr != nil && loadErr != context.Canceled && loadErr != context.DeadlineExceeded {
			return out, loadErr
		}
		if err := coord.CompleteSession(m.CoordURL, sid); err != nil {
			return out, err
		}
		log.Printf("[s-dw] session %s completed; mirror disabled", sid)

		if err := cold.WaitReady(m.WarmupURL, 120*time.Second); err != nil {
			return out, err
		}
		m.PostLoadSettle()
		log.Printf("[s-dw] post-load settle %dms for session %s", m.PostLoadSettleMs, sid)

		snap, err := FetchState(m.WarmupURL)
		if err != nil {
			return out, err
		}
		out.ShadowCountAtProbe = snap.Warmkit.ShadowCount
		out.WarmkitState = string(snap.Warmkit.State)
		log.Printf("[s-dw] pre-probe state session=%s shadowCount=%d warmkit=%s",
			sid, out.ShadowCountAtProbe, out.WarmkitState)
		return out, nil
	}, true)
}

type overheadSide struct {
	name    string
	enabled bool
	ratio   float64
}

func overheadSideSpec(mirrorOff bool) overheadSide {
	if mirrorOff {
		return overheadSide{"overhead-mirror-off", false, 0}
	}
	return overheadSide{"overhead-mirror-on", true, 1.0}
}

func overheadSideCanCollect(count, maxRuns int) bool {
	return count < maxRuns
}

func pickOverheadSide(attempt, offCount, onCount, maxRuns int) (overheadSide, bool) {
	preferOff := attempt%2 == 1
	try := func(mirrorOff bool) (overheadSide, bool) {
		n := onCount
		if mirrorOff {
			n = offCount
		}
		if !overheadSideCanCollect(n, maxRuns) {
			return overheadSide{}, false
		}
		return overheadSideSpec(mirrorOff), true
	}
	if preferOff {
		if side, ok := try(true); ok {
			return side, true
		}
		return try(false)
	}
	if side, ok := try(false); ok {
		return side, true
	}
	return try(true)
}

func overheadCollectionDone(offRes, onRes Result, m profile.Manifest) bool {
	if len(offRes.Runs) < m.H4Runs || len(onRes.Runs) < m.H4Runs {
		return false
	}
	if !m.ProfileOnHighCV {
		return true
	}
	return offRes.CVPass && onRes.CVPass
}

// RunOverhead compares response latency through proxy with mirror off vs on.
// Runs alternate off/on between attempts; each attempt uses identical warmup
// (full ramp/steady/down) before measuring steady-window RTT only.
func RunOverhead(ctx context.Context, m profile.Manifest) ([]Result, error) {
	offRecords := make([]RunRecord, 0, m.H4MaxRuns)
	onRecords := make([]RunRecord, 0, m.H4MaxRuns)
	offRunP95 := make([]float64, 0, m.H4MaxRuns)
	onRunP95 := make([]float64, 0, m.H4MaxRuns)
	offBlocks := make([]float64, 0, m.H4MaxRuns*m.H4SteadySec)
	onBlocks := make([]float64, 0, m.H4MaxRuns*m.H4SteadySec)
	offRTTs := make([]float64, 0)
	onRTTs := make([]float64, 0)

	maxPerSide := m.H4MaxRuns
	if !m.ProfileOnHighCV {
		maxPerSide = m.H4Runs
	}
	maxAttempts := maxPerSide * 2

	log.Printf("[overhead] alternating off/on | target=%d/side max=%d | warmup+measure per attempt",
		m.H4Runs, m.H4MaxRuns)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		side, ok := pickOverheadSide(attempt, len(offRecords), len(onRecords), maxPerSide)
		if !ok {
			break
		}
		log.Printf("[overhead] attempt %d | %s | off=%d on=%d valid",
			attempt, side.name, len(offRecords), len(onRecords))

		runIndex := len(offRecords) + 1
		if side.enabled {
			runIndex = len(onRecords) + 1
		}
		sample, err := runOverheadAttempt(ctx, m, side, attempt, runIndex)
		if err != nil {
			return nil, err
		}
		if side.enabled {
			onRecords = append(onRecords, sample.record)
			onRunP95 = append(onRunP95, sample.runP95)
			onBlocks = append(onBlocks, sample.blocks...)
			onRTTs = append(onRTTs, sample.rtts...)
		} else {
			offRecords = append(offRecords, sample.record)
			offRunP95 = append(offRunP95, sample.runP95)
			offBlocks = append(offBlocks, sample.blocks...)
			offRTTs = append(offRTTs, sample.rtts...)
		}

		if len(offRecords) >= m.H4Runs && len(onRecords) >= m.H4Runs {
			offRes := buildOverheadResult("overhead-mirror-off", offRecords, offRunP95, offRTTs, offBlocks, m)
			onRes := buildOverheadResult("overhead-mirror-on", onRecords, onRunP95, onRTTs, onBlocks, m)
			if overheadCollectionDone(offRes, onRes, m) {
				_ = coord.BaselineMirror(m.CoordURL, m.ActiveInst)
				return []Result{offRes, onRes}, nil
			}
			if len(offRecords) >= m.H4MaxRuns && len(onRecords) >= m.H4MaxRuns {
				log.Printf("[overhead] CV fail off=%s on=%s after max runs", offRes.CVPassReason, onRes.CVPassReason)
				_ = coord.BaselineMirror(m.CoordURL, m.ActiveInst)
				return []Result{offRes, onRes}, nil
			}
			log.Printf("[overhead] CV fail off=%s on=%s, continuing alternating runs",
				offRes.CVPassReason, onRes.CVPassReason)
		}
	}

	offRes := buildOverheadResult("overhead-mirror-off", offRecords, offRunP95, offRTTs, offBlocks, m)
	onRes := buildOverheadResult("overhead-mirror-on", onRecords, onRunP95, onRTTs, onBlocks, m)
	_ = coord.BaselineMirror(m.CoordURL, m.ActiveInst)
	return []Result{offRes, onRes}, nil
}

// RunH4Overhead is deprecated; use RunOverhead.
func RunH4Overhead(ctx context.Context, m profile.Manifest) ([]Result, error) {
	return RunOverhead(ctx, m)
}

type overheadSample struct {
	record RunRecord
	runP95 float64
	blocks []float64
	rtts   []float64
}

func runOverheadAttempt(ctx context.Context, m profile.Manifest, side overheadSide, globalAttempt, runIndex int) (overheadSample, error) {
	cfg := warmkit.MirrorConfig{
		Enabled:          side.enabled,
		Ratio:            side.ratio,
		TargetInstanceID: m.TargetInst,
		ActiveInstanceID: m.ActiveInst,
	}
	if err := coord.MirrorConfig(m.CoordURL, cfg); err != nil {
		return overheadSample{}, err
	}

	m.Cooldown()
	lp := m.H4LoadProfile()
	totalSec := m.TotalH4LoadSec()
	timeout := time.Duration(totalSec+30) * time.Second

	warmCtx, warmCancel := context.WithTimeout(ctx, timeout)
	if err := loadgen.RunPhased(warmCtx, m.ProxyURL, m.Profile, lp); err != nil {
		warmCancel()
		return overheadSample{}, fmt.Errorf("[%s] warmup: %w", side.name, err)
	}
	warmCancel()
	m.Cooldown()

	measCtx, measCancel := context.WithTimeout(ctx, timeout)
	measStart := time.Now()
	rttRes, err := loadgen.CollectRTTPhased(measCtx, m.ProxyURL, m.Profile, lp)
	measCancel()
	measMs := time.Since(measStart).Milliseconds()
	if err != nil && len(rttRes.Steady) == 0 {
		return overheadSample{}, err
	}
	blocks := loadgen.BlockMedians(rttRes.Steady, m.RPS)
	if len(blocks) == 0 {
		return overheadSample{}, fmt.Errorf("[%s] no overhead steady blocks", side.name)
	}
	blockSum := stats.SummarizeFilteredWithSeed(blocks, blocks, nil, m.Seed)
	runP95 := blockSum.P95

	return overheadSample{
		record: RunRecord{
			RunIndex:          runIndex,
			Attempt:           globalAttempt,
			LoadgenDurationMs: measMs,
			Probe: probe.Sample{
				WorkloadNs: int64(runP95),
				E2ENs:      int64(blockSum.P50),
			},
		},
		runP95: runP95,
		blocks: blocks,
		rtts:   intsToFloats(rttRes.All),
	}, nil
}

func runIndependent(m profile.Manifest, name string, warmup func(context.Context) (WarmupOutcome, error), trackReady bool) (Result, error) {
	records := make([]RunRecord, 0, m.MaxRuns)
	values := make([]float64, 0, m.MaxRuns)
	e2e := make([]float64, 0, m.MaxRuns)
	maxValid := m.Runs
	if m.ProfileOnHighCV {
		maxValid = m.MaxRuns
	}
	maxAttempts := m.MaxAttempts()
	tracker := progress.NewTracker(m, name)
	tracker.LogScenarioStart()

	for attempt := 1; attempt <= maxAttempts && len(records) < maxValid; attempt++ {
		tracker.LogRunStart(len(records), attempt)
		if err := cold.ResetWarmup(m.WarmupURL); err != nil {
			return Result{}, err
		}
		time.Sleep(200 * time.Millisecond)
		m.Cooldown()
		runCtx := context.Background()
		warmupOut, err := warmup(runCtx)
		if err != nil {
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
			log.Printf("[%s] attempt %d: invalid pre-probe state (readyOk=%v indexCold=%v)",
				name, attempt, readyOK, snap.Workload.IndexCold)
			continue
		}
		s, err := probe.TFirst(m.WarmupURL, m.Profile)
		if err != nil {
			log.Printf("[%s] attempt %d: probe failed: %v", name, attempt, err)
			continue
		}
		rec := RunRecord{
			RunIndex:           len(records) + 1,
			Attempt:            attempt,
			Probe:              s,
			State:              snap,
			ReadyOK:            readyOK,
			SessionID:          warmupOut.SessionID,
			ShadowCountAtProbe: warmupOut.ShadowCountAtProbe,
			PostLoadSettleMs:   warmupOut.PostLoadSettleMs,
			LoadgenDurationMs:  warmupOut.LoadgenDurationMs,
		}
		if !warmupOut.SessionReadyAt.IsZero() {
			rec.SessionReadyAt = warmupOut.SessionReadyAt.Format(time.RFC3339)
		}
		if warmupOut.WarmkitState != "" {
			rec.Warmkit = warmkit.State(warmupOut.WarmkitState)
		} else if snap.Warmkit.State != "" {
			rec.Warmkit = snap.Warmkit.State
		}
		records = append(records, rec)
		values = append(values, float64(s.WorkloadNs))
		e2e = append(e2e, float64(s.E2ENs))
		tracker.LogRunDone(len(records), s.WorkloadNs, s.E2ENs, warmupOut.ShadowCountAtProbe)
		m.Cooldown()

		if len(records) >= m.Runs {
			res := buildResult(name, records, values, e2e, m)
			if res.CVPass || len(records) >= maxValid || !m.ProfileOnHighCV {
				return res, nil
			}
			tracker.LogCVRetry(len(records), res.CVPassReason)
		}
	}
	if len(records) == 0 {
		return Result{}, fmt.Errorf("[%s] no valid runs after %d attempts", name, maxAttempts)
	}
	res := buildResult(name, records, values, e2e, m)
	return res, nil
}

func buildResult(name string, records []RunRecord, values, e2e []float64, m profile.Manifest) Result {
	filtered, outliers := stats.FilterIQRIndexed(values)
	sum := stats.SummarizeFilteredWithSeed(values, filtered, outliers, m.Seed)
	e2eFiltered, e2eOutliers := stats.FilterIQRIndexed(e2e)
	e2eSum := stats.SummarizeFilteredWithSeed(e2e, e2eFiltered, e2eOutliers, m.Seed)
	cv := stats.EvaluateCV(sum, m.CVThresholdForScenario(name))
	displayName, description := ScenarioInfo(name)
	return Result{
		Scenario:     name,
		DisplayName:  displayName,
		Description:  description,
		Summary:      sum,
		E2ESummary:   e2eSum,
		Values:       values,
		E2EValues:    e2e,
		Runs:         records,
		CVPass:       cv.Pass,
		CVPassReason: cv.Reason,
	}
}

func buildOverheadResult(name string, records []RunRecord, runP95Values, allRTTValues, allBlockValues []float64, m profile.Manifest) Result {
	filtered, outliers := stats.FilterIQRIndexed(runP95Values)
	sum := stats.SummarizeFilteredWithSeed(runP95Values, filtered, outliers, m.Seed)
	blockFiltered, blockOutliers := stats.FilterIQRIndexed(allBlockValues)
	blockSum := stats.SummarizeFilteredWithSeed(allBlockValues, blockFiltered, blockOutliers, m.Seed)
	values := runP95Values
	if len(records) < 5 {
		sum = blockSum
		values = allBlockValues
	}
	cv := stats.EvaluateCV(sum, m.CVThreshold)
	displayName, description := ScenarioInfo(name)
	return Result{
		Scenario:     name,
		DisplayName:  displayName,
		Description:  description,
		Summary:      sum,
		BlockSummary: blockSum,
		Values:       values,
		E2EValues:    allRTTValues,
		BlockValues:  allBlockValues,
		Runs:         records,
		CVPass:       cv.Pass,
		CVPassReason: cv.Reason,
		Hypothesis:   "overhead: run-level p95 mirror-on vs mirror-off on proxy /work",
	}
}

func ScenarioInfo(name string) (string, string) {
	switch name {
	case "s0-control":
		return "Без прогрева", "Холодный инстанс: первый GET /work строит in-memory hashmap."
	case "s-ref":
		return "Ручной прогрев", "Перед первым боевым GET /work выполняется POST /warmup с тем же профилем."
	case "s-dw":
		return "Динамический прогрев", "Инстанс прогревается зеркалированным GET-трафиком через СДПС."
	case "overhead-mirror-off":
		return "Зеркалирование выключено", "Базовая задержка ответа через mirror-proxy."
	case "overhead-mirror-on":
		return "Зеркалирование включено", "Задержка ответа при асинхронном mirror-трафике на warmup."
	case "h4-mirror-off":
		return ScenarioInfo("overhead-mirror-off")
	case "h4-mirror-on":
		return ScenarioInfo("overhead-mirror-on")
	default:
		return name, ""
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

// EvaluateOverhead compares mirror-on vs mirror-off p95 on proxy /work.
func EvaluateOverhead(off, on Result) string {
	if overheadObserved(on) <= overheadObserved(off)*1.05 {
		return "pass"
	}
	return "fail"
}

// EvaluateH4 is deprecated; use EvaluateOverhead.
func EvaluateH4(off, on Result) string {
	return EvaluateOverhead(off, on)
}

func overheadObserved(r Result) float64 {
	if len(r.Runs) > 0 {
		return r.Summary.P50
	}
	return r.Summary.P95
}

func h4Observed(r Result) float64 {
	return overheadObserved(r)
}

// EvaluateHypotheses compares scenario medians for H1/H2/H3 notes.
func EvaluateHypotheses(s0, sRef, sDw Result) map[string]string {
	notes := map[string]string{}
	if sDw.Summary.P50 <= s0.Summary.P50/2 {
		notes["H1"] = "pass"
	} else {
		notes["H1"] = "fail"
	}
	if sDw.Summary.P50 <= sRef.Summary.P50*2.0 {
		notes["H2"] = "pass"
	} else {
		notes["H2"] = "fail"
	}
	refCIBound := sRef.Summary.P50CIHigh * 2.0
	if sDw.Summary.P50CIHigh <= refCIBound {
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

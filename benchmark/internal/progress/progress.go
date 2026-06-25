package progress

import (
	"fmt"
	"log"
	"time"

	"github.com/itmo-vkr/dwss/benchmark/internal/profile"
)

// Tracker logs per-scenario run progress and ETA.
type Tracker struct {
	Scenario   string
	StartedAt  time.Time
	RunSec     int
	TargetRuns int
	MaxRuns    int
}

// NewTracker creates a progress tracker for one scenario.
func NewTracker(m profile.Manifest, scenario string) *Tracker {
	maxRuns := m.Runs
	if m.ProfileOnHighCV {
		maxRuns = m.MaxRuns
	}
	return &Tracker{
		Scenario:   scenario,
		StartedAt:  time.Now(),
		RunSec:     EstimateRunSec(m, scenario),
		TargetRuns: m.Runs,
		MaxRuns:    maxRuns,
	}
}

// EstimateRunSec approximates wall-clock seconds for one valid run.
func EstimateRunSec(m profile.Manifest, scenario string) int {
	cooldownSec := m.CooldownMs / 1000
	if cooldownSec < 1 {
		cooldownSec = 1
	}
	switch scenario {
	case "s-dw":
		return m.TotalLoadSec() + m.PostLoadSettleMs/1000 + cooldownSec*2 + 15
	case "s-ref":
		return cooldownSec*2 + 5
	case "overhead-mirror-off", "overhead-mirror-on", "h4-mirror-off", "h4-mirror-on":
		// Warmup + measure, each with full phased profile.
		return 2*m.TotalH4LoadSec() + 2*cooldownSec + 5
	default:
		return cooldownSec*2 + 3
	}
}

// EstimateTotalSec sums ETA for a list of scenarios at max runs.
func EstimateTotalSec(m profile.Manifest, scenarios []string) int {
	total := 0
	maxRuns := m.Runs
	if m.ProfileOnHighCV {
		maxRuns = m.MaxRuns
	}
	for _, scenario := range scenarios {
		switch scenario {
		case "overhead", "h4-overhead":
			// Alternating off/on; up to H4MaxRuns per side.
			perAttempt := EstimateRunSec(m, "overhead-mirror-off")
			total += perAttempt * m.H4MaxRuns * 2
		default:
			total += EstimateRunSec(m, scenario) * maxRuns
		}
	}
	return total
}

// LogScenarioStart logs when a scenario begins.
func (t *Tracker) LogScenarioStart() {
	log.Printf("[bench] scenario=%s | target runs=%d max=%d | ~%s per run",
		t.Scenario, t.TargetRuns, t.MaxRuns, FormatDuration(time.Duration(t.RunSec)*time.Second))
}

// LogRunStart logs before a run attempt begins.
func (t *Tracker) LogRunStart(validCount, attempt int) {
	remaining := t.MaxRuns - validCount
	if remaining < 1 {
		remaining = 1
	}
	elapsed := time.Since(t.StartedAt)
	eta := time.Duration(t.RunSec*remaining) * time.Second
	log.Printf("[%s] run %d/%d (attempt %d) | elapsed %s | ETA scenario ~%s",
		t.Scenario, validCount+1, t.TargetRuns, attempt, FormatDuration(elapsed), FormatDuration(eta))
}

// LogRunDone logs after a valid run is recorded.
func (t *Tracker) LogRunDone(validCount int, workloadNs, e2eNs int64, shadowCount int64) {
	elapsed := time.Since(t.StartedAt)
	remaining := t.MaxRuns - validCount
	if remaining < 0 {
		remaining = 0
	}
	eta := time.Duration(t.RunSec*remaining) * time.Second
	if shadowCount > 0 {
		log.Printf("[%s] run %d/%d valid | probe workload=%.2fms e2e=%.2fms | shadowCount=%d | elapsed %s | ETA ~%s",
			t.Scenario, validCount, t.TargetRuns, float64(workloadNs)/1e6, float64(e2eNs)/1e6,
			shadowCount, FormatDuration(elapsed), FormatDuration(eta))
		return
	}
	log.Printf("[%s] run %d/%d valid | probe workload=%.2fms e2e=%.2fms | elapsed %s | ETA ~%s",
		t.Scenario, validCount, t.TargetRuns, float64(workloadNs)/1e6, float64(e2eNs)/1e6,
		FormatDuration(elapsed), FormatDuration(eta))
}

// LogCVRetry logs adaptive run collection.
func (t *Tracker) LogCVRetry(validCount int, reason string) {
	log.Printf("[%s] CV fail (%s), collecting run %d..%d", t.Scenario, reason, validCount+1, t.MaxRuns)
}

// FormatDuration renders a human-readable duration.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// LogPlan logs the full benchmark plan with total ETA.
func LogPlan(scenarios []string, totalSec int) {
	log.Printf("[bench] scenarios=%v | estimated total ~%s (max runs, adaptive CV)",
		scenarios, FormatDuration(time.Duration(totalSec)*time.Second))
}

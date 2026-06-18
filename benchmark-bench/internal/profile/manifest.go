package profile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/itmo-vkr/dwss/internal/envcfg"
	"github.com/itmo-vkr/dwss/warmkit"
)

const ProtocolVersion = "2"

const minRuns = 5

// RunMeta captures reproducibility metadata written into every manifest.
type RunMeta struct {
	ProtocolVersion string    `json:"protocolVersion"`
	StartedAt       time.Time `json:"startedAt"`
	GOMAXPROCS      int       `json:"gomaxprocs"`
	GitCommit       string    `json:"gitCommit"`
}

// Manifest holds experiment environment metadata.
type Manifest struct {
	RunMeta
	Seed            int64                   `json:"seed"`
	Samples         int                     `json:"samples"`
	Runs            int                     `json:"runs"`
	MaxRuns         int                     `json:"maxRuns"`
	ProfileOnHighCV bool                    `json:"profileOnHighCv"`
	Profile         warmkit.WorkloadProfile `json:"profile"`
	ProxyURL        string                  `json:"proxyUrl"`
	WarmupURL       string                  `json:"warmupUrl"`
	CoordURL        string                  `json:"coordUrl"`
	ActiveURL       string                  `json:"activeUrl"`
	RPS             int                     `json:"rps"`
	LoadSec         int                     `json:"loadDurationSec"`
	CVThreshold     float64                 `json:"cvThreshold"`
	CooldownMs      int                     `json:"cooldownMs"`
	TargetInst      string                  `json:"targetInstanceId"`
	ActiveInst      string                  `json:"activeInstanceId"`
	ReadyAfter      int                     `json:"readyAfter"`
	H4LoadSec       int                     `json:"h4LoadDurationSec"`
}

// LoadFromEnv builds manifest using required env vars.
func LoadFromEnv() (Manifest, error) {
	var m Manifest
	var err error
	m.RunMeta = collectRunMeta()
	if m.Seed, err = envcfg.RequiredInt64("DWSS_BENCH_SEED"); err != nil {
		return m, err
	}
	if m.Samples, err = envcfg.RequiredInt("DWSS_BENCH_SAMPLES"); err != nil {
		return m, err
	}
	if m.Runs, err = envcfg.RequiredInt("DWSS_BENCH_RUNS"); err != nil {
		return m, err
	}
	if m.Runs < minRuns {
		return m, fmt.Errorf("DWSS_BENCH_RUNS=%d: minimum %d required for valid statistics", m.Runs, minRuns)
	}
	if m.MaxRuns, err = envcfg.OptionalInt("DWSS_BENCH_MAX_RUNS", m.Runs); err != nil {
		return m, err
	}
	if m.MaxRuns < m.Runs {
		return m, fmt.Errorf("DWSS_BENCH_MAX_RUNS=%d must be >= DWSS_BENCH_RUNS=%d", m.MaxRuns, m.Runs)
	}
	profileHighCV, err := envcfg.OptionalBool("DWSS_BENCH_PROFILE_ON_HIGH_CV", false)
	if err != nil {
		return m, err
	}
	m.ProfileOnHighCV = profileHighCV
	if m.ProxyURL, err = envcfg.Required("DWSS_BENCH_PROXY_URL"); err != nil {
		return m, err
	}
	if m.WarmupURL, err = envcfg.Required("DWSS_BENCH_WARMUP_URL"); err != nil {
		return m, err
	}
	if m.CoordURL, err = envcfg.Required("DWSS_BENCH_COORD_URL"); err != nil {
		return m, err
	}
	if m.ActiveURL, err = envcfg.Required("DWSS_BENCH_ACTIVE_URL"); err != nil {
		return m, err
	}
	if m.RPS, err = envcfg.RequiredInt("DWSS_BENCH_RPS"); err != nil {
		return m, err
	}
	if m.LoadSec, err = envcfg.RequiredInt("DWSS_BENCH_LOAD_DURATION_SEC"); err != nil {
		return m, err
	}
	cvStr, err := envcfg.Required("DWSS_BENCH_CV_THRESHOLD")
	if err != nil {
		return m, err
	}
	m.CVThreshold, err = strconv.ParseFloat(cvStr, 64)
	if err != nil {
		return m, err
	}
	if m.CooldownMs, err = envcfg.RequiredInt("DWSS_BENCH_COOLDOWN_MS"); err != nil {
		return m, err
	}
	if m.TargetInst, err = envcfg.Required("DWSS_BENCH_TARGET_INSTANCE"); err != nil {
		return m, err
	}
	if m.ActiveInst, err = envcfg.Required("DWSS_BENCH_ACTIVE_INSTANCE"); err != nil {
		return m, err
	}
	if m.ReadyAfter, err = envcfg.RequiredInt("DWSS_BENCH_READY_AFTER"); err != nil {
		return m, err
	}
	if m.H4LoadSec, err = envcfg.RequiredInt("DWSS_BENCH_H4_LOAD_DURATION_SEC"); err != nil {
		return m, err
	}
	m.Profile = warmkit.WorkloadProfile{Seed: m.Seed, Samples: m.Samples}
	return m, nil
}

// ValidateWarmupReadyAfter checks bench READY_AFTER matches warmup service config.
func (m Manifest) ValidateWarmupReadyAfter() error {
	resp, err := http.Get(m.WarmupURL + "/state")
	if err != nil {
		return fmt.Errorf("warmup state check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("warmup state status %d", resp.StatusCode)
	}
	var snap struct {
		Warmkit warmkit.MetricsSnapshot `json:"warmkit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return err
	}
	if snap.Warmkit.ReadyAfter != m.ReadyAfter {
		return fmt.Errorf(
			"DWSS_BENCH_READY_AFTER=%d but warmup readyAfter=%d (sync with DWSS_WARMUP_READY_AFTER)",
			m.ReadyAfter, snap.Warmkit.ReadyAfter,
		)
	}
	return nil
}

func collectRunMeta() RunMeta {
	commit := "unknown"
	if out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}
	return RunMeta{
		ProtocolVersion: ProtocolVersion,
		StartedAt:       time.Now().UTC(),
		GOMAXPROCS:      runtime.GOMAXPROCS(0),
		GitCommit:       commit,
	}
}

// Cooldown sleeps between independent cold runs.
func (m Manifest) Cooldown() {
	if m.CooldownMs <= 0 {
		return
	}
	time.Sleep(time.Duration(m.CooldownMs) * time.Millisecond)
}

// EnrichFromFlags overrides instance ids when CLI flags are set.
func (m *Manifest) EnrichFromFlags(target, active string, ready int) {
	if target != "" {
		m.TargetInst = target
	}
	if active != "" {
		m.ActiveInst = active
	}
	if ready > 0 {
		m.ReadyAfter = ready
	}
}

// MaxAttempts returns the upper bound on run attempts for a scenario.
func (m Manifest) MaxAttempts() int {
	if m.ProfileOnHighCV {
		return m.MaxRuns * 2
	}
	return m.Runs * 2
}

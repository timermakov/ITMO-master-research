package profile

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/itmo-vkr/dwss/internal/envcfg"
	"github.com/itmo-vkr/dwss/warmkit"
)

const ProtocolVersion = "2"

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
	Seed        int64                   `json:"seed"`
	Samples     int                     `json:"samples"`
	Runs        int                     `json:"runs"`
	Profile     warmkit.WorkloadProfile `json:"profile"`
	ProxyURL    string                  `json:"proxyUrl"`
	WarmupURL   string                  `json:"warmupUrl"`
	CoordURL    string                  `json:"coordUrl"`
	ActiveURL   string                  `json:"activeUrl"`
	RPS         int                     `json:"rps"`
	LoadSec     int                     `json:"loadDurationSec"`
	CVThreshold float64                 `json:"cvThreshold"`
	CooldownMs  int                     `json:"cooldownMs"`
	TargetInst  string                  `json:"targetInstanceId"`
	ActiveInst  string                  `json:"activeInstanceId"`
	ReadyAfter  int                     `json:"readyAfter"`
	H4LoadSec   int                     `json:"h4LoadDurationSec"`
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

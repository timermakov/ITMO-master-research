package profile

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/itmo-vkr/dwss/benchmark-bench/internal/loadgen"
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
	GitDirty        bool      `json:"gitDirty"`
	GitStatus       string    `json:"gitStatus,omitempty"`
	GoVersion       string    `json:"goVersion"`
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	NumCPU          int       `json:"numCpu"`
	DockerVersion   string    `json:"dockerVersion,omitempty"`
	DockerImages    []string  `json:"dockerImages,omitempty"`
}

// Manifest holds experiment environment metadata.
type Manifest struct {
	RunMeta
	Seed             int64                   `json:"seed"`
	Samples          int                     `json:"samples"`
	Runs             int                     `json:"runs"`
	MaxRuns          int                     `json:"maxRuns"`
	ProfileOnHighCV  bool                    `json:"profileOnHighCv"`
	Profile          warmkit.WorkloadProfile `json:"profile"`
	ProxyURL         string                  `json:"proxyUrl"`
	WarmupURL        string                  `json:"warmupUrl"`
	CoordURL         string                  `json:"coordUrl"`
	ActiveURL        string                  `json:"activeUrl"`
	RPS              int                     `json:"rps"`
	RampUpSec        int                     `json:"rampUpSec"`
	SteadySec        int                     `json:"steadySec"`
	RampDownSec      int                     `json:"rampDownSec"`
	H4SteadySec      int                     `json:"h4SteadySec"`
	H4Runs           int                     `json:"h4Runs"`
	H4MaxRuns        int                     `json:"h4MaxRuns"`
	CVThreshold      float64                 `json:"cvThreshold"`
	CVThresholdS0    float64                 `json:"cvThresholdS0"`
	CVThresholdSRef  float64                 `json:"cvThresholdSRef"`
	CVThresholdSDw   float64                 `json:"cvThresholdSDw"`
	CooldownMs       int                     `json:"cooldownMs"`
	EnvProfileName   string                  `json:"envProfileName,omitempty"`
	TargetInst       string                  `json:"targetInstanceId"`
	ActiveInst       string                  `json:"activeInstanceId"`
	ReadyAfter       int                     `json:"readyAfter"`
	ReadyAfterActual int                     `json:"readyAfterActual,omitempty"`
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
	if m.RampUpSec, err = envcfg.RequiredInt("DWSS_BENCH_RAMP_UP_SEC"); err != nil {
		return m, err
	}
	if m.SteadySec, err = envcfg.RequiredInt("DWSS_BENCH_STEADY_SEC"); err != nil {
		return m, err
	}
	if m.RampDownSec, err = envcfg.RequiredInt("DWSS_BENCH_RAMP_DOWN_SEC"); err != nil {
		return m, err
	}
	if m.H4SteadySec, err = envcfg.RequiredInt("DWSS_BENCH_H4_STEADY_SEC"); err != nil {
		return m, err
	}
	if m.H4Runs, err = envcfg.OptionalInt("DWSS_BENCH_H4_RUNS", minRuns); err != nil {
		return m, err
	}
	if m.H4Runs < 1 {
		return m, fmt.Errorf("DWSS_BENCH_H4_RUNS must be >= 1")
	}
	if m.H4MaxRuns, err = envcfg.OptionalInt("DWSS_BENCH_H4_MAX_RUNS", m.H4Runs); err != nil {
		return m, err
	}
	if m.H4MaxRuns < m.H4Runs {
		return m, fmt.Errorf("DWSS_BENCH_H4_MAX_RUNS=%d must be >= DWSS_BENCH_H4_RUNS=%d", m.H4MaxRuns, m.H4Runs)
	}
	if m.RampUpSec < 1 {
		return m, fmt.Errorf("DWSS_BENCH_RAMP_UP_SEC must be >= 1")
	}
	if m.SteadySec < 5 {
		return m, fmt.Errorf("DWSS_BENCH_STEADY_SEC must be >= 5 (steady phase for metrics)")
	}
	cvStr, err := envcfg.Required("DWSS_BENCH_CV_THRESHOLD")
	if err != nil {
		return m, err
	}
	m.CVThreshold, err = strconv.ParseFloat(cvStr, 64)
	if err != nil {
		return m, err
	}
	if m.CVThresholdS0, err = optionalFloat("DWSS_BENCH_CV_THRESHOLD_S0", m.CVThreshold); err != nil {
		return m, err
	}
	if m.CVThresholdSRef, err = optionalFloat("DWSS_BENCH_CV_THRESHOLD_S_REF", m.CVThreshold); err != nil {
		return m, err
	}
	if m.CVThresholdSDw, err = optionalFloat("DWSS_BENCH_CV_THRESHOLD_S_DW", m.CVThreshold); err != nil {
		return m, err
	}
	if m.CooldownMs, err = envcfg.RequiredInt("DWSS_BENCH_COOLDOWN_MS"); err != nil {
		return m, err
	}
	if m.EnvProfileName, err = optionalString("DWSS_BENCH_ENV_PROFILE", ""); err != nil {
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
	m.Profile = warmkit.WorkloadProfile{Seed: m.Seed, Samples: m.Samples}
	return m, nil
}

func optionalString(key, defaultValue string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func optionalFloat(key string, defaultValue float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	out, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("environment variable %q: %w", key, err)
	}
	return out, nil
}

// LoadProfile builds the S_dw / production load shape.
func (m Manifest) LoadProfile() loadgen.Profile {
	return loadgen.Profile{
		MaxRPS:      m.RPS,
		RampUpSec:   m.RampUpSec,
		SteadySec:   m.SteadySec,
		RampDownSec: m.RampDownSec,
	}
}

// H4LoadProfile builds H4 load shape (same ramp, H4-specific steady window).
func (m Manifest) H4LoadProfile() loadgen.Profile {
	return loadgen.Profile{
		MaxRPS:      m.RPS,
		RampUpSec:   m.RampUpSec,
		SteadySec:   m.H4SteadySec,
		RampDownSec: m.RampDownSec,
	}
}

// TotalLoadSec is ramp + steady + ramp-down for S_dw.
func (m Manifest) TotalLoadSec() int {
	return m.RampUpSec + m.SteadySec + m.RampDownSec
}

// TotalH4LoadSec is total duration for H4 phases.
func (m Manifest) TotalH4LoadSec() int {
	return m.RampUpSec + m.H4SteadySec + m.RampDownSec
}

// ValidateWarmupReadyAfter checks bench READY_AFTER matches warmup service config.
func (m *Manifest) ValidateWarmupReadyAfter() error {
	resp, err := http.Get(m.WarmupURL + "/state")
	if err != nil {
		return fmt.Errorf("warmup state check: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("warmup state status %d", resp.StatusCode)
	}
	var snap struct {
		Warmkit warmkit.MetricsSnapshot `json:"warmkit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return err
	}
	m.ReadyAfterActual = snap.Warmkit.ReadyAfter
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
	status := gitStatus()
	return RunMeta{
		ProtocolVersion: ProtocolVersion,
		StartedAt:       time.Now().UTC(),
		GOMAXPROCS:      runtime.GOMAXPROCS(0),
		GitCommit:       commit,
		GitDirty:        status != "",
		GitStatus:       status,
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		NumCPU:          runtime.NumCPU(),
		DockerVersion:   commandOutput("docker", "--version"),
		DockerImages:    dockerImages(),
	}
}

func gitStatus() string {
	return commandOutput("git", "status", "--short")
}

func dockerImages() []string {
	out := commandOutput("docker", "compose", "-f", "../deploy/docker-compose.yml", "images", "-q")
	if out == "" {
		out = commandOutput("docker", "compose", "-f", "deploy/docker-compose.yml", "images", "-q")
	}
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	images := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			images = append(images, line)
		}
	}
	return images
}

func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m Manifest) CVThresholdForScenario(scenario string) float64 {
	switch scenario {
	case "s0-control":
		return m.CVThresholdS0
	case "s-ref":
		return m.CVThresholdSRef
	case "s-dw":
		return m.CVThresholdSDw
	default:
		return m.CVThreshold
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

package config

import (
	"time"

	"github.com/itmo-vkr/dwss/internal/envcfg"
	"github.com/itmo-vkr/dwss/warmkit"
)

const (
	EnvHTTPAddr           = "DWSS_HTTP_ADDR"
	EnvRole               = "DWSS_ROLE"
	EnvInstanceID         = "DWSS_INSTANCE_ID"
	EnvMmapFile           = "DWSS_MMAP_FILE"
	EnvMmapSizeMB         = "DWSS_MMAP_SIZE_MB"
	EnvIndexKeys          = "DWSS_INDEX_KEYS"
	EnvServiceName        = "DWSS_SERVICE_NAME"
	EnvCoordination       = "DWSS_COORDINATION"
	EnvZKEndpoints        = "DWSS_ZK_ENDPOINTS"
	EnvFlagdOFREPURL      = "DWSS_FLAGD_OFREP_URL"
	EnvCoordURL           = "DWSS_COORD_URL"
	EnvFlagdPollMS        = "DWSS_FLAGD_POLL_MS"
	EnvFlagdHTTPTimeoutMS = "DWSS_FLAGD_HTTP_TIMEOUT_MS"
	EnvReadyAfter         = "DWSS_WARMUP_READY_AFTER"
	EnvWarmupTimeout      = "DWSS_WARMUP_TIMEOUT_SEC"
	EnvShadowKeep         = "DWSS_SHADOW_METRICS_KEEP"
)

// Settings holds LoadedService configuration from environment.
type Settings struct {
	HTTPAddr         string
	Role             string
	InstanceID       string
	MmapFile         string
	MmapSizeMB       int
	IndexKeys        int
	ServiceName      string
	Coordination     warmkit.Backend
	ZKEndpoints      []string
	FlagdOFREPURL    string
	CoordURL         string
	FlagdPoll        time.Duration
	FlagdHTTPTimeout time.Duration
	ReadyAfter       int
	WarmupTimeout    time.Duration
	ShadowKeep       int
}

// Load reads required environment variables.
func Load() (Settings, error) {
	var s Settings
	var err error
	if s.HTTPAddr, err = envcfg.Required(EnvHTTPAddr); err != nil {
		return s, err
	}
	if s.Role, err = envcfg.Required(EnvRole); err != nil {
		return s, err
	}
	if s.InstanceID, err = envcfg.Required(EnvInstanceID); err != nil {
		return s, err
	}
	if s.MmapFile, err = envcfg.Required(EnvMmapFile); err != nil {
		return s, err
	}
	if s.MmapSizeMB, err = envcfg.RequiredInt(EnvMmapSizeMB); err != nil {
		return s, err
	}
	if s.IndexKeys, err = envcfg.RequiredInt(EnvIndexKeys); err != nil {
		return s, err
	}
	if s.ServiceName, err = envcfg.Required(EnvServiceName); err != nil {
		return s, err
	}
	backend, err := envcfg.Required(EnvCoordination)
	if err != nil {
		return s, err
	}
	if s.Coordination, err = warmkit.ParseBackend(backend); err != nil {
		return s, err
	}
	switch s.Coordination {
	case warmkit.BackendZooKeeper:
		if s.ZKEndpoints, err = envcfg.RequiredCSV(EnvZKEndpoints); err != nil {
			return s, err
		}
	case warmkit.BackendOpenFeature:
		if s.FlagdOFREPURL, err = envcfg.Required(EnvFlagdOFREPURL); err != nil {
			return s, err
		}
		if s.CoordURL, err = envcfg.Required(EnvCoordURL); err != nil {
			return s, err
		}
		pollMS, err := envcfg.RequiredInt(EnvFlagdPollMS)
		if err != nil {
			return s, err
		}
		s.FlagdPoll = time.Duration(pollMS) * time.Millisecond
		timeoutMS, err := envcfg.RequiredInt(EnvFlagdHTTPTimeoutMS)
		if err != nil {
			return s, err
		}
		s.FlagdHTTPTimeout = time.Duration(timeoutMS) * time.Millisecond
	}
	if s.ReadyAfter, err = envcfg.RequiredInt(EnvReadyAfter); err != nil {
		return s, err
	}
	if s.WarmupTimeout, err = envcfg.RequiredDurationFromSeconds(EnvWarmupTimeout); err != nil {
		return s, err
	}
	if s.ShadowKeep, err = envcfg.RequiredInt(EnvShadowKeep); err != nil {
		return s, err
	}
	return s, nil
}

// WarmkitConfig builds warmkit.Config from settings.
func (s Settings) WarmkitConfig() warmkit.Config {
	return warmkit.Config{
		ServiceName:       s.ServiceName,
		InstanceID:        s.InstanceID,
		ListenAddr:        s.HTTPAddr,
		Role:              s.Role,
		ReadyAfter:        s.ReadyAfter,
		WarmupTimeout:     s.WarmupTimeout,
		ShadowMetricsKeep: s.ShadowKeep,
	}
}

// OFConfig builds OpenFeature client settings.
func (s Settings) OFConfig() warmkit.OFConfig {
	return warmkit.OFConfig{
		OFREPURL:       s.FlagdOFREPURL,
		CoordinatorURL: s.CoordURL,
		PollInterval:   s.FlagdPoll,
		HTTPTimeout:    s.FlagdHTTPTimeout,
	}
}

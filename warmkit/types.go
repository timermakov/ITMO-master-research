package warmkit

// State is the warmup instance lifecycle state.
type State string

const (
	StateStarting   State = "Starting"
	StateRegistered State = "Registered"
	StateWarming    State = "Warming"
	StateReady      State = "Ready"
	StateActive     State = "Active"
	StateFailed     State = "Failed"
)

// WorkloadProfile describes a reproducible /work or /warmup request.
type WorkloadProfile struct {
	Seed    int64 `json:"seed"`
	Samples int   `json:"samples"`
}

// MetricsSnapshot is exported warmup metrics.
type MetricsSnapshot struct {
	State            State `json:"state"`
	ShadowCount      int64 `json:"shadowCount"`
	ShadowLatencyP95 int64 `json:"shadowLatencyP95Ns"`
	ReadyAfter       int   `json:"readyAfter"`
}

// MirrorConfig is stored in ZK at /config/{service}/mirror.
type MirrorConfig struct {
	Enabled            bool    `json:"enabled"`
	Ratio              float64 `json:"ratio"`
	TargetInstanceID   string  `json:"targetInstanceId"`
	ActiveInstanceID   string  `json:"activeInstanceId"`
	SessionID          string  `json:"sessionId,omitempty"`
	UpdatedAtUnixMilli int64   `json:"updatedAt"`
}

// InstanceRecord is stored in ZK at /services/{service}/instances/{id}.
type InstanceRecord struct {
	Addr      string `json:"addr"`
	State     State  `json:"state"`
	Role      string `json:"role"`
	StartedAt int64  `json:"startedAt"`
}

const (
	HeaderShadow = "X-Warmup-Shadow"
	ShadowValue  = "1"
)

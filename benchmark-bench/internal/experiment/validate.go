package experiment

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

// StateSnapshot is workload + warmkit state from GET /state.
type StateSnapshot struct {
	Workload struct {
		MmapCold bool `json:"mmapCold"`
		AppCold  bool `json:"appCold"`
	} `json:"workload"`
	Warmkit warmkit.MetricsSnapshot `json:"warmkit"`
}

// FetchState reads LoadedService /state.
func FetchState(warmupURL string) (StateSnapshot, error) {
	var snap StateSnapshot
	resp, err := http.Get(warmupURL + "/state")
	if err != nil {
		return snap, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return snap, fmt.Errorf("state status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return snap, err
	}
	return snap, nil
}

// WaitWarmWorkload polls until mmap and app caches are warm.
func WaitWarmWorkload(warmupURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := FetchState(warmupURL)
		if err == nil && !snap.Workload.MmapCold && !snap.Workload.AppCold {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("workload not warm within %s", timeout)
}

// ValidPreProbe checks scenario-specific readiness before T_first probe.
func ValidPreProbe(scenario string, snap StateSnapshot, readyOK bool) bool {
	switch scenario {
	case "s0-control":
		return snap.Workload.MmapCold || snap.Workload.AppCold
	case "s-ref":
		return !snap.Workload.MmapCold && !snap.Workload.AppCold
	case "s-dw":
		return readyOK && !snap.Workload.MmapCold && !snap.Workload.AppCold
	default:
		return true
	}
}

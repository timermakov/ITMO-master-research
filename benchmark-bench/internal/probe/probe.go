package probe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

// Sample holds primary (workload) and secondary (E2E) latency for one probe.
type Sample struct {
	WorkloadNs int64 `json:"workloadNs"`
	E2ENs      int64 `json:"e2eNs"`
}

// TFirst issues one non-shadow GET /work and returns workload + E2E latency.
func TFirst(baseURL string, p warmkit.WorkloadProfile) (Sample, error) {
	u := fmt.Sprintf("%s/work?seed=%d&samples=%d", baseURL, p.Seed, p.Samples)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return Sample{}, err
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Sample{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	e2e := time.Since(start).Nanoseconds()
	if resp.StatusCode != http.StatusOK {
		return Sample{}, fmt.Errorf("probe status %d", resp.StatusCode)
	}
	var out struct {
		DurationNs int64 `json:"duration_ns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Sample{E2ENs: e2e}, nil
	}
	workload := out.DurationNs
	if workload <= 0 {
		workload = e2e
	}
	return Sample{WorkloadNs: workload, E2ENs: e2e}, nil
}

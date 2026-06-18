package cold

import (
	"fmt"
	"net/http"
	"time"
)

// ResetWarmup returns the warmup instance to cold workload + Registered warmkit state.
func ResetWarmup(warmupURL string) error {
	req, err := http.NewRequest(http.MethodPost, warmupURL+"/reset", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reset status %d", resp.StatusCode)
	}
	return nil
}

// WaitReady polls /readyz until 200 or timeout.
func WaitReady(warmupURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(warmupURL + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("readyz timeout")
}

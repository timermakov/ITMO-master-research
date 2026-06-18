package loadgen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

// Run sends steady GET /work traffic through the proxy for the given duration.
func Run(ctx context.Context, proxyURL string, profile warmkit.WorkloadProfile, rps int, durationSec int) error {
	if rps <= 0 || durationSec <= 0 {
		return fmt.Errorf("invalid loadgen params")
	}
	interval := time.Second / time.Duration(rps)
	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	client := &http.Client{Timeout: interval * 2}
	path := fmt.Sprintf("/work?seed=%d&samples=%d", profile.Seed, profile.Samples)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyURL+path, nil)
			if err != nil {
				return err
			}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

// CollectRTT samples end-to-end GET /work latency through the proxy (active path).
func CollectRTT(ctx context.Context, proxyURL string, profile warmkit.WorkloadProfile, rps int, durationSec int) ([]int64, error) {
	if rps <= 0 || durationSec <= 0 {
		return nil, fmt.Errorf("invalid loadgen params")
	}
	interval := time.Second / time.Duration(rps)
	deadline := time.Now().Add(time.Duration(durationSec) * time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	client := &http.Client{Timeout: interval * 2}
	path := fmt.Sprintf("/work?seed=%d&samples=%d", profile.Seed, profile.Samples)
	samples := make([]int64, 0, rps*durationSec)
	for {
		select {
		case <-ctx.Done():
			return samples, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return samples, nil
			}
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyURL+path, nil)
			if err != nil {
				return samples, err
			}
			resp, err := client.Do(req)
			if err != nil {
				return samples, err
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			samples = append(samples, time.Since(start).Nanoseconds())
		}
	}
}

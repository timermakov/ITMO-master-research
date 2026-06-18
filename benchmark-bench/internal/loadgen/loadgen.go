package loadgen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

// RunPhased sends GET /work through the proxy following ramp-up → steady → ramp-down.
func RunPhased(ctx context.Context, proxyURL string, wp warmkit.WorkloadProfile, lp Profile) error {
	if lp.MaxRPS <= 0 || lp.TotalSec() <= 0 {
		return fmt.Errorf("invalid load profile")
	}
	path := fmt.Sprintf("/work?seed=%d&samples=%d", wp.Seed, wp.Samples)
	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()
	for {
		elapsed := time.Since(start).Seconds()
		if elapsed >= float64(lp.TotalSec()) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rps := lp.TargetRPS(elapsed)
		if rps <= 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		interval := time.Second / time.Duration(rps)
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
		time.Sleep(interval)
	}
}

// RTTResult holds steady-phase samples and the full run for audit.
type RTTResult struct {
	Steady []int64
	All    []int64
}

// CollectRTTPhased samples E2E latency; only Steady slice is used for H4 statistics.
func CollectRTTPhased(ctx context.Context, proxyURL string, wp warmkit.WorkloadProfile, lp Profile) (RTTResult, error) {
	out := RTTResult{}
	if lp.MaxRPS <= 0 || lp.TotalSec() <= 0 {
		return out, fmt.Errorf("invalid load profile")
	}
	path := fmt.Sprintf("/work?seed=%d&samples=%d", wp.Seed, wp.Samples)
	client := &http.Client{Timeout: 30 * time.Second}
	start := time.Now()
	for {
		elapsed := time.Since(start).Seconds()
		if elapsed >= float64(lp.TotalSec()) {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		rps := lp.TargetRPS(elapsed)
		if rps <= 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		interval := time.Second / time.Duration(rps)
		t0 := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyURL+path, nil)
		if err != nil {
			return out, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return out, err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		rtt := time.Since(t0).Nanoseconds()
		out.All = append(out.All, rtt)
		if lp.InSteady(elapsed) {
			out.Steady = append(out.Steady, rtt)
		}
		sleep := interval - time.Since(t0)
		if sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

// Run sends steady GET /work traffic through the proxy for the given duration.
// Deprecated: use RunPhased with steady-only profile.
func Run(ctx context.Context, proxyURL string, profile warmkit.WorkloadProfile, rps int, durationSec int) error {
	return RunPhased(ctx, proxyURL, profile, Profile{MaxRPS: rps, SteadySec: durationSec})
}

// CollectRTT samples end-to-end GET /work latency through the proxy (active path).
// Deprecated: use CollectRTTPhased.
func CollectRTT(ctx context.Context, proxyURL string, profile warmkit.WorkloadProfile, rps int, durationSec int) ([]int64, error) {
	res, err := CollectRTTPhased(ctx, proxyURL, profile, Profile{MaxRPS: rps, SteadySec: durationSec})
	if err != nil {
		return nil, err
	}
	return res.Steady, nil
}

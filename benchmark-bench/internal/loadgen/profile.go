package loadgen

import "math"

// Profile describes a three-phase load shape: ramp-up → steady → ramp-down.
// Metrics must be taken only during the steady phase (see Habr load-testing practice).
type Profile struct {
	MaxRPS      int
	RampUpSec   int
	SteadySec   int
	RampDownSec int
}

// TotalSec returns full load test duration.
func (p Profile) TotalSec() int {
	return p.RampUpSec + p.SteadySec + p.RampDownSec
}

// InSteady reports whether elapsed seconds from test start fall in the steady window.
func (p Profile) InSteady(elapsedSec float64) bool {
	if elapsedSec < float64(p.RampUpSec) {
		return false
	}
	return elapsedSec < float64(p.RampUpSec+p.SteadySec)
}

// Phase reports the load-test phase at elapsedSec from test start.
func (p Profile) Phase(elapsedSec float64) string {
	if elapsedSec < float64(p.RampUpSec) {
		return "ramp-up"
	}
	if elapsedSec < float64(p.RampUpSec+p.SteadySec) {
		return "steady"
	}
	return "ramp-down"
}

// TargetRPS returns intended RPS at elapsedSec from test start (linear ramp-up/down).
func (p Profile) TargetRPS(elapsedSec float64) int {
	if elapsedSec < 0 {
		return 0
	}
	total := float64(p.TotalSec())
	if elapsedSec >= total {
		return 0
	}
	if p.RampUpSec > 0 && elapsedSec < float64(p.RampUpSec) {
		rps := int(math.Round(float64(p.MaxRPS) * elapsedSec / float64(p.RampUpSec)))
		if rps < 1 && elapsedSec > 0 {
			return 1
		}
		return rps
	}
	steadyEnd := float64(p.RampUpSec + p.SteadySec)
	if elapsedSec < steadyEnd {
		return p.MaxRPS
	}
	if p.RampDownSec <= 0 {
		return 0
	}
	remaining := steadyEnd + float64(p.RampDownSec) - elapsedSec
	rps := int(math.Round(float64(p.MaxRPS) * remaining / float64(p.RampDownSec)))
	if rps < 0 {
		return 0
	}
	return rps
}

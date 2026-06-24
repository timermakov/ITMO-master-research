package loadgen

import "testing"

func TestTargetRPSRampUp(t *testing.T) {
	p := Profile{MaxRPS: 100, RampUpSec: 10, SteadySec: 60, RampDownSec: 10}
	if r := p.TargetRPS(0); r != 0 {
		t.Fatalf("start rps=%d", r)
	}
	if r := p.TargetRPS(5); r != 50 {
		t.Fatalf("mid ramp rps=%d want 50", r)
	}
	if r := p.TargetRPS(10); r != 100 {
		t.Fatalf("end ramp rps=%d", r)
	}
	if r := p.TargetRPS(40); r != 100 {
		t.Fatalf("steady rps=%d", r)
	}
}

func TestTargetRPSRampDown(t *testing.T) {
	p := Profile{MaxRPS: 100, RampUpSec: 10, SteadySec: 60, RampDownSec: 10}
	if r := p.TargetRPS(75); r != 50 {
		t.Fatalf("mid down rps=%d want 50", r)
	}
	if r := p.TargetRPS(80); r != 0 {
		t.Fatalf("end rps=%d", r)
	}
}

func TestInSteady(t *testing.T) {
	p := Profile{MaxRPS: 50, RampUpSec: 30, SteadySec: 300, RampDownSec: 30}
	if p.InSteady(0) || p.InSteady(29.9) {
		t.Fatal("should not be steady during ramp-up")
	}
	if !p.InSteady(30) || !p.InSteady(329.9) {
		t.Fatal("should be steady")
	}
	if p.InSteady(330) {
		t.Fatal("should not be steady during ramp-down")
	}
}

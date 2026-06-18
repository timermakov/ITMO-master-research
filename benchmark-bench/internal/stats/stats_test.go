package stats

import (
	"math"
	"testing"
)

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil)
	if s.N != 0 {
		t.Fatalf("expected n=0, got %d", s.N)
	}
}

func TestSummarizeBasic(t *testing.T) {
	values := []float64{100, 200, 300, 400, 500}
	s := Summarize(values)
	if s.N != 5 {
		t.Fatalf("n=%d", s.N)
	}
	if s.P50 != 300 {
		t.Fatalf("p50=%f", s.P50)
	}
	if s.CILow >= s.CIHigh || s.CILow == s.CIHigh {
		t.Fatalf("ci collapsed: %f %f", s.CILow, s.CIHigh)
	}
}

func TestBootstrapCI(t *testing.T) {
	data := []float64{10, 20, 30, 40, 50}
	low, high := bootstrapCI(data, 500, 0.95)
	if low >= high {
		t.Fatalf("ci invalid: %f %f", low, high)
	}
	if low > 30 || high < 30 {
		t.Fatalf("ci should bracket mean: %f %f", low, high)
	}
}

func TestFilterIQR(t *testing.T) {
	values := []float64{1, 2, 3, 4, 100}
	filtered := FilterIQR(values)
	if len(filtered) >= len(values) {
		t.Fatalf("expected outlier removal")
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if p := percentile(sorted, 0.50); p != 3 {
		t.Fatalf("p50=%f", p)
	}
	if math.IsNaN(percentile(nil, 0.5)) {
		t.Fatal("nan")
	}
}

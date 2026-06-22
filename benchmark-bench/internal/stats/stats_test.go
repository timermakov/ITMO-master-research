package stats

import (
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
	if s.NFiltered != 5 {
		t.Fatalf("nFiltered=%d", s.NFiltered)
	}
	if s.P50 != 300 {
		t.Fatalf("p50=%f", s.P50)
	}
	if s.CILow >= s.CIHigh || s.CILow == s.CIHigh {
		t.Fatalf("mean ci collapsed: %f %f", s.CILow, s.CIHigh)
	}
	if s.P50CILow >= s.P50CIHigh {
		t.Fatalf("p50 ci collapsed: %f %f", s.P50CILow, s.P50CIHigh)
	}
}

func TestBootstrapCI(t *testing.T) {
	data := []float64{10, 20, 30, 40, 50}
	low, high := bootstrapCI(data, 500, 0.95, bootstrapMean, 42)
	if low >= high {
		t.Fatalf("ci invalid: %f %f", low, high)
	}
	if low > 30 || high < 30 {
		t.Fatalf("ci should bracket mean: %f %f", low, high)
	}
	low2, high2 := bootstrapCI(data, 500, 0.95, bootstrapMean, 42)
	if low != low2 || high != high2 {
		t.Fatalf("ci should be reproducible for the same seed: [%f, %f] vs [%f, %f]", low, high, low2, high2)
	}
}

func TestFilterIQR(t *testing.T) {
	values := []float64{1, 2, 3, 4, 100}
	filtered, outliers := FilterIQRIndexed(values)
	if len(filtered) >= len(values) {
		t.Fatalf("expected outlier removal")
	}
	if len(outliers) == 0 {
		t.Fatalf("expected outlier indexes")
	}
}

func TestFilterIQRSmallN(t *testing.T) {
	values := []float64{1, 2, 3}
	filtered, outliers := FilterIQRIndexed(values)
	if len(filtered) != 3 || len(outliers) != 0 {
		t.Fatalf("small n should pass through")
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if p := Percentile(sorted, 0.50); p != 3 {
		t.Fatalf("p50=%f", p)
	}
	if p := Percentile(nil, 0.5); p != 0 {
		t.Fatalf("empty p50=%f", p)
	}
}

func TestEvaluateCV(t *testing.T) {
	s := Summarize([]float64{100, 102, 98, 101, 99})
	v := EvaluateCV(s, 5)
	if !v.Pass {
		t.Fatalf("expected pass, got %s", v.Reason)
	}
	s2 := Summary{N: 3, NFiltered: 3, CV: 1}
	v2 := EvaluateCV(s2, 5)
	if v2.Pass {
		t.Fatalf("expected fail for n_filtered < 5")
	}
}

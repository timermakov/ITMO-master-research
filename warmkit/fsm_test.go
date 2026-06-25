package warmkit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFSMTransitions(t *testing.T) {
	f := NewFSM()
	if f.State() != StateStarting {
		t.Fatalf("got %s", f.State())
	}
	if err := f.Transition(StateRegistered); err != nil {
		t.Fatal(err)
	}
	if err := f.Transition(StateWarming); err != nil {
		t.Fatal(err)
	}
	if err := f.Transition(StateReady); err != nil {
		t.Fatal(err)
	}
	if err := f.Transition(StateActive); err != nil {
		t.Fatal(err)
	}
	if err := f.Transition(StateWarming); err == nil {
		t.Fatal("expected invalid transition from Active")
	}
}

func TestShadowMetricsP95(t *testing.T) {
	m := NewShadowMetrics(100)
	for i := 0; i < 10; i++ {
		m.Record(time.Duration(i+1) * time.Millisecond)
	}
	if m.Count() != 10 {
		t.Fatalf("count %d", m.Count())
	}
	if m.P95Ns() <= 0 {
		t.Fatal("expected positive p95")
	}
}

func TestEngineWarmupReady(t *testing.T) {
	reg := NewMemoryRegistry()
	reg.SetMirror(MirrorConfig{
		Enabled:          true,
		Ratio:            1,
		TargetInstanceID: "warmup-1",
	})
	cfg := Config{
		ServiceName:       "loaded-service",
		InstanceID:        "warmup-1",
		ListenAddr:        "warmup:8081",
		Role:              "warmup",
		ReadyAfter:        2,
		WarmupTimeout:     5 * time.Second,
		ShadowMetricsKeep: 100,
	}
	eng := NewEngine(cfg, reg, nil)
	ctx := context.Background()
	if err := eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if eng.State() == StateWarming || eng.State() == StateReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := eng.ShadowMiddleware(stub)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/work", nil)
		req.Header.Set(HeaderShadow, ShadowValue)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
	}
	if eng.State() != StateReady {
		t.Fatalf("state %s", eng.State())
	}
}

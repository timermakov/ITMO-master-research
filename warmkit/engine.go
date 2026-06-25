package warmkit

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Config holds warmkit engine settings (from env via caller).
type Config struct {
	ServiceName       string
	InstanceID        string
	ListenAddr        string
	Role              string
	ReadyAfter        int
	WarmupTimeout     time.Duration
	ShadowMetricsKeep int
}

// Engine controls warmup lifecycle.
type Engine interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	ResetBenchmark(ctx context.Context) error
	State() State
	Snapshot() MetricsSnapshot
	ReadinessHandler() http.HandlerFunc
	ShadowMiddleware(next http.Handler) http.Handler
}

type engine struct {
	cfg    Config
	reg    Registry
	fsm    *FSM
	shadow *ShadowMetrics
	log    *slog.Logger
	mu     sync.RWMutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewEngine creates a warmup engine.
func NewEngine(cfg Config, reg Registry, log *slog.Logger) Engine {
	if log == nil {
		log = slog.Default()
	}
	keep := cfg.ShadowMetricsKeep
	return &engine{
		cfg:    cfg,
		reg:    reg,
		fsm:    NewFSM(),
		shadow: NewShadowMetrics(keep),
		log:    log,
	}
}

func (e *engine) State() State {
	return e.fsm.State()
}

func (e *engine) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		State:            e.fsm.State(),
		ShadowCount:      e.shadow.Count(),
		ShadowLatencyP95: e.shadow.P95Ns(),
		ReadyAfter:       e.cfg.ReadyAfter,
	}
}

func (e *engine) Start(ctx context.Context) error {
	if e.cfg.Role != "warmup" {
		_ = e.fsm.Transition(StateActive)
		return nil
	}

	rec := InstanceRecord{
		Addr:      e.cfg.ListenAddr,
		State:     StateRegistered,
		Role:      e.cfg.Role,
		StartedAt: time.Now().UnixMilli(),
	}
	if err := e.reg.RegisterInstance(ctx, e.cfg.ServiceName, e.cfg.InstanceID, rec); err != nil {
		return fmt.Errorf("register instance: %w", err)
	}
	if err := e.fsm.Transition(StateRegistered); err != nil {
		return err
	}
	_ = e.reg.UpdateInstanceState(ctx, e.cfg.ServiceName, e.cfg.InstanceID, StateRegistered)

	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	e.wg.Add(1)
	go e.runMirrorWatch(runCtx)

	return nil
}

func (e *engine) runMirrorWatch(ctx context.Context) {
	defer e.wg.Done()
	ch, err := e.reg.WatchMirror(ctx, e.cfg.ServiceName)
	if err != nil {
		e.log.Error("watch mirror", slog.String("err", err.Error()))
		_ = e.fsm.Transition(StateFailed)
		return
	}

	timeout := time.NewTimer(e.cfg.WarmupTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case cfg, ok := <-ch:
			if !ok {
				return
			}
			e.handleMirrorConfig(ctx, cfg)
			if e.fsm.State() == StateReady {
				timeout.Stop()
			}
		case <-timeout.C:
			if e.fsm.State() != StateReady && e.fsm.State() != StateActive {
				_ = e.fsm.Transition(StateFailed)
				_ = e.reg.UpdateInstanceState(ctx, e.cfg.ServiceName, e.cfg.InstanceID, StateFailed)
				e.log.Error("warmup timeout")
				return
			}
		}
	}
}

func (e *engine) handleMirrorConfig(ctx context.Context, cfg MirrorConfig) {
	if cfg.TargetInstanceID == e.cfg.InstanceID && cfg.Enabled && cfg.Ratio > 0 {
		if e.fsm.State() == StateRegistered {
			_ = e.fsm.Transition(StateWarming)
			_ = e.reg.UpdateInstanceState(ctx, e.cfg.ServiceName, e.cfg.InstanceID, StateWarming)
		}
	}
	if cfg.ActiveInstanceID == e.cfg.InstanceID && e.fsm.State() == StateReady {
		_ = e.fsm.Transition(StateActive)
		_ = e.reg.UpdateInstanceState(ctx, e.cfg.ServiceName, e.cfg.InstanceID, StateActive)
	}
	if e.shadow.Count() >= int64(e.cfg.ReadyAfter) && e.fsm.State() == StateWarming {
		_ = e.fsm.Transition(StateReady)
		_ = e.reg.UpdateInstanceState(ctx, e.cfg.ServiceName, e.cfg.InstanceID, StateReady)
	}
}

func (e *engine) ResetBenchmark(ctx context.Context) error {
	if e.cfg.Role != "warmup" {
		return nil
	}
	e.mu.Lock()
	keep := e.cfg.ShadowMetricsKeep
	e.shadow = NewShadowMetrics(keep)
	e.fsm.Force(StateRegistered)
	e.mu.Unlock()
	return e.reg.UpdateInstanceState(ctx, e.cfg.ServiceName, e.cfg.InstanceID, StateRegistered)
}

func (e *engine) Stop(ctx context.Context) error {
	if e.cancel != nil {
		e.cancel()
	}
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return e.reg.Close()
}

func (e *engine) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := e.fsm.State()
		if st == StateReady || st == StateActive {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(string(st)))
	}
}

func (e *engine) ShadowMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsShadowRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "shadow mutations forbidden", http.StatusForbidden)
			return
		}
		if e.fsm.State() == StateRegistered {
			_ = e.fsm.Transition(StateWarming)
			_ = e.reg.UpdateInstanceState(r.Context(), e.cfg.ServiceName, e.cfg.InstanceID, StateWarming)
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		e.shadow.Record(time.Since(start))
		if e.shadow.Count() >= int64(e.cfg.ReadyAfter) && e.fsm.State() == StateWarming {
			_ = e.fsm.Transition(StateReady)
			_ = e.reg.UpdateInstanceState(r.Context(), e.cfg.ServiceName, e.cfg.InstanceID, StateReady)
		}
	})
}

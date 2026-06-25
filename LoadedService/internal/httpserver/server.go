package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/itmo-vkr/dwss/LoadedService/internal/workload"
	"github.com/itmo-vkr/dwss/warmkit"
)

// Server serves LoadedService HTTP API.
type Server struct {
	log     *slog.Logger
	work    *workload.Engine
	warmkit warmkit.Engine
	mux     *http.ServeMux
	http    *http.Server
	addr    string
	role    string
}

// New creates an HTTP server.
func New(addr, role string, log *slog.Logger, work *workload.Engine, wk warmkit.Engine) *Server {
	s := &Server{
		log:     log,
		work:    work,
		warmkit: wk,
		addr:    addr,
		role:    role,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	if role == "warmup" && wk != nil {
		mux.HandleFunc("GET /readyz", wk.ReadinessHandler())
	}
	mux.HandleFunc("GET /work", s.handleWork)
	mux.HandleFunc("POST /warmup", s.handleWarmup)
	mux.HandleFunc("POST /reset", s.handleReset)
	mux.HandleFunc("GET /state", s.handleState)
	handler := http.Handler(mux)
	if wk != nil {
		handler = wk.ShadowMiddleware(mux)
	}
	s.mux = mux
	s.http = &http.Server{Addr: addr, Handler: handler}
	return s
}

func (s *Server) Start() error {
	s.log.Info("http server starting", slog.String("addr", s.addr), slog.String("role", s.role))
	go func() {
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error("http server error", slog.String("err", err.Error()))
		}
	}()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleWork(w http.ResponseWriter, r *http.Request) {
	p, err := parseProfile(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err)
		return
	}
	ns := s.work.RunProfile(p)
	s.respondJSON(w, http.StatusOK, map[string]any{"duration_ns": ns})
}

func (s *Server) handleWarmup(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err)
		return
	}
	var p warmkit.WorkloadProfile
	if err := json.Unmarshal(body, &p); err != nil {
		s.respondError(w, http.StatusBadRequest, err)
		return
	}
	s.work.WarmProfile(p)
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "warmed"})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if err := s.work.ResetCold(); err != nil {
		s.respondError(w, http.StatusInternalServerError, err)
		return
	}
	if s.warmkit != nil {
		if err := s.warmkit.ResetBenchmark(r.Context()); err != nil {
			s.respondError(w, http.StatusInternalServerError, err)
			return
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{"workload": s.work.SnapshotState()}
	if s.warmkit != nil {
		resp["warmkit"] = s.warmkit.Snapshot()
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func parseProfile(r *http.Request) (warmkit.WorkloadProfile, error) {
	var p warmkit.WorkloadProfile
	seedStr := r.URL.Query().Get("seed")
	samplesStr := r.URL.Query().Get("samples")
	if seedStr == "" || samplesStr == "" {
		return p, fmt.Errorf("seed and samples query params required")
	}
	var err error
	if p.Seed, err = parseInt64(seedStr); err != nil {
		return p, err
	}
	if p.Samples, err = parseInt(samplesStr); err != nil {
		return p, err
	}
	return p, nil
}

func parseInt64(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscan(s, &v)
	return v, err
}

func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscan(s, &v)
	return v, err
}

func (s *Server) respondJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) respondError(w http.ResponseWriter, code int, err error) {
	s.log.Error("request error", slog.Int("status", code), slog.String("err", err.Error()))
	s.respondJSON(w, code, map[string]string{"error": err.Error()})
}

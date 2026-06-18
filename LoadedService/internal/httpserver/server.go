package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/itmo-vkr/loadedservice/internal/bench"
	"github.com/itmo-vkr/loadedservice/internal/mmapstore"
)

// Config holds HTTP server and benchmark configuration.
type Config struct {
	Addr           string
	DefaultSamples int
}

// Server provides minimal REST API to demonstrate cold start effects.
type Server struct {
	cfg   Config
	log   *slog.Logger
	store *mmapstore.Store
	http  *http.Server
}

func New(cfg Config, logger *slog.Logger, store *mmapstore.Store) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{cfg: cfg, log: logger, store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/warmup", s.handleWarmup)
	mux.HandleFunc("/bench", s.handleBench)
	mux.HandleFunc("/config", s.handleConfig)
	s.http = &http.Server{Addr: cfg.Addr, Handler: mux}
	return s
}

func (s *Server) Start() error {
	s.log.Info("http server starting", slog.String("addr", s.cfg.Addr))
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"addr":            s.cfg.Addr,
		"default_samples": s.cfg.DefaultSamples,
		"mapped_size_mb":  s.store.SizeMB(),
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleWarmup(w http.ResponseWriter, r *http.Request) {
	if err := s.store.TouchSequential(); err != nil {
		s.respondError(w, http.StatusInternalServerError, err)
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "warmed"})
}

func (s *Server) handleBench(w http.ResponseWriter, r *http.Request) {
	samples := s.cfg.DefaultSamples
	if v := r.URL.Query().Get("samples"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			samples = n
		}
	}

	runs := 1
	if v := r.URL.Query().Get("runs"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			runs = n
		}
	}

	// Default seed is fixed for reproducibility unless explicitly overridden.
	seed := int64(42)
	if v := r.URL.Query().Get("seed"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed = n
		}
	}

	res, err := bench.RunColdWarmMany(s.store, samples, runs, seed)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err)
		return
	}
	s.respondJSON(w, http.StatusOK, res)
}

func (s *Server) respondJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) respondError(w http.ResponseWriter, code int, err error) {
	s.log.Error("request error", slog.Int("status", code), slog.String("err", err.Error()))
	s.respondJSON(w, code, map[string]string{"error": fmt.Sprintf("%v", err)})
}

// Wait blocks until the server is closed; convenient for main.
func (s *Server) Wait() {
	// Busy wait with periodic sleep; ListenAndServe runs in goroutine.
	for {
		time.Sleep(24 * time.Hour)
	}
}

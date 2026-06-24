package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/itmo-vkr/dwss/internal/envcfg"
	"github.com/itmo-vkr/dwss/warmkit"
)

const (
	envHTTPAddr     = "DWSS_COORD_HTTP_ADDR"
	envZKEndpoints  = "DWSS_COORD_ZK_ENDPOINTS"
	envServiceName  = "DWSS_COORD_SERVICE_NAME"
	envRampInterval = "DWSS_COORD_RAMP_INTERVAL_SEC"
	envRampSteps    = "DWSS_COORD_RAMP_STEPS"
	envHoldMaxSec   = "DWSS_COORD_HOLD_MAX_SEC"
	envShutdownSec  = "DWSS_COORD_SHUTDOWN_SEC"
)

type session struct {
	ID               string `json:"id"`
	TargetInstanceID string `json:"targetInstanceId"`
	ActiveInstanceID string `json:"activeInstanceId"`
	ReadyAfter       int    `json:"readyAfter"`
	Status           string `json:"status"`
}

type coordinator struct {
	log      *slog.Logger
	conn     *zk.Conn
	service  string
	interval time.Duration
	steps    []float64
	holdMax  time.Duration
	mu       sync.Mutex
	sessions map[string]*session
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr, err := envcfg.Required(envHTTPAddr)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	endpoints, err := envcfg.RequiredCSV(envZKEndpoints)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	service, err := envcfg.Required(envServiceName)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	intervalSec, err := envcfg.RequiredInt(envRampInterval)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	stepsRaw, err := envcfg.Required(envRampSteps)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	shutdownSec, err := envcfg.RequiredInt(envShutdownSec)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	holdMaxSec, err := envcfg.OptionalInt(envHoldMaxSec, 120)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}

	steps, err := parseSteps(stepsRaw)
	if err != nil {
		logger.Error("ramp steps", slog.String("err", err.Error()))
		os.Exit(1)
	}

	conn, _, err := zk.Connect(endpoints, time.Duration(shutdownSec)*time.Second)
	if err != nil {
		logger.Error("zk", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer conn.Close()

	c := &coordinator{
		log:      logger,
		conn:     conn,
		service:  service,
		interval: time.Duration(intervalSec) * time.Second,
		steps:    steps,
		holdMax:  time.Duration(holdMaxSec) * time.Second,
		sessions: make(map[string]*session),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /v1/warmup/sessions", c.createSession)
	mux.HandleFunc("GET /v1/warmup/sessions/{id}", c.getSession)
	mux.HandleFunc("POST /v1/warmup/sessions/{id}/complete", c.completeSession)
	mux.HandleFunc("DELETE /v1/warmup/sessions/{id}", c.deleteSession)
	mux.HandleFunc("PUT /v1/mirror/config", c.setMirrorConfig)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", slog.String("err", err.Error()))
		}
	}()
	logger.Info("coordinator listening", slog.String("addr", addr))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shCtx, shCancel := context.WithTimeout(context.Background(), time.Duration(shutdownSec)*time.Second)
	defer shCancel()
	_ = srv.Shutdown(shCtx)
}

func parseSteps(raw string) ([]float64, error) {
	parts := strings.Split(raw, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (c *coordinator) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetInstanceID string `json:"targetInstanceId"`
		ReadyAfter       int    `json:"readyAfter"`
		ActiveInstanceID string `json:"activeInstanceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := time.Now().Format("20060102150405.000")
	s := &session{
		ID:               id,
		TargetInstanceID: req.TargetInstanceID,
		ActiveInstanceID: req.ActiveInstanceID,
		ReadyAfter:       req.ReadyAfter,
		Status:           "running",
	}
	c.mu.Lock()
	c.sessions[id] = s
	c.mu.Unlock()

	go c.runRamp(context.Background(), id, req.TargetInstanceID, req.ActiveInstanceID, req.ReadyAfter)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(s)
}

func (c *coordinator) runRamp(ctx context.Context, sessionID, target, active string, readyAfter int) {
	cfg := warmkit.MirrorConfig{
		Enabled:          true,
		Ratio:            0,
		TargetInstanceID: target,
		ActiveInstanceID: active,
		SessionID:        sessionID,
	}
	_ = warmkit.WriteMirrorConfig(c.conn, c.service, cfg)
	holdDeadline := time.Now().Add(c.holdMax)

	for _, ratio := range c.steps {
		select {
		case <-ctx.Done():
			return
		default:
		}
		cfg.Ratio = ratio
		_ = warmkit.WriteMirrorConfig(c.conn, c.service, cfg)
		c.log.Info("ramp", slog.Float64("ratio", ratio), slog.String("session", sessionID))
		time.Sleep(c.interval)

		if c.instanceReady(target) {
			break
		}
	}

	cfg.Ratio = 1.0
	cfg.Enabled = true
	for !c.instanceReady(target) && time.Now().Before(holdDeadline) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = warmkit.WriteMirrorConfig(c.conn, c.service, cfg)
		c.log.Info("hold mirror", slog.String("session", sessionID))
		time.Sleep(c.interval)
	}

	_ = warmkit.WriteMirrorConfig(c.conn, c.service, cfg)

	c.mu.Lock()
	if s, ok := c.sessions[sessionID]; ok {
		s.Status = "holding"
	}
	c.mu.Unlock()
	c.log.Info("session holding mirror until complete", slog.String("session", sessionID))
}

func (c *coordinator) completeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c.mu.Lock()
	s, ok := c.sessions[id]
	if !ok {
		c.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if s.Status == "completed" {
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
		return
	}
	target := s.TargetInstanceID
	active := s.ActiveInstanceID
	s.Status = "completed"
	c.mu.Unlock()

	cfg := warmkit.MirrorConfig{
		Enabled:          false,
		Ratio:            0,
		TargetInstanceID: target,
		ActiveInstanceID: active,
	}
	if err := warmkit.WriteMirrorConfig(c.conn, c.service, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.log.Info("session completed, mirror disabled", slog.String("session", id))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

func (c *coordinator) instanceReady(instanceID string) bool {
	path := "/services/" + c.service + "/instances/" + instanceID
	b, _, err := c.conn.Get(path)
	if err != nil {
		return false
	}
	var rec warmkit.InstanceRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return false
	}
	return rec.State == warmkit.StateReady || rec.State == warmkit.StateActive
}

func (c *coordinator) getSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c.mu.Lock()
	s, ok := c.sessions[id]
	c.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

func (c *coordinator) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c.mu.Lock()
	delete(c.sessions, id)
	c.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (c *coordinator) setMirrorConfig(w http.ResponseWriter, r *http.Request) {
	var req warmkit.MirrorConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := warmkit.WriteMirrorConfig(c.conn, c.service, req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(req)
}

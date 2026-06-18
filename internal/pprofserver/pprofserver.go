package pprofserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"

	"github.com/itmo-vkr/dwss/internal/envcfg"
)

const (
	EnvEnabled        = "DWSS_PPROF_ENABLED"
	EnvHTTPAddr       = "DWSS_PPROF_HTTP_ADDR"
	EnvProfileSeconds = "DWSS_PPROF_PROFILE_SECONDS"
)

// Server serves pprof on a dedicated mux when enabled.
type Server struct {
	log  *slog.Logger
	http *http.Server
}

// StartFromEnv starts pprof server if DWSS_PPROF_ENABLED is true.
func StartFromEnv(ctx context.Context, log *slog.Logger) (*Server, error) {
	enabled, err := envcfg.RequiredBool(EnvEnabled)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	addr, err := envcfg.Required(EnvHTTPAddr)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &Server{
		log: log,
		http: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
	go func() {
		if err := srv.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("pprof server", slog.String("err", err.Error()))
		}
	}()
	log.Info("pprof listening", slog.String("addr", addr))
	return srv, nil
}

// ProfileURL builds profile URL using env seconds.
func ProfileURL() (string, error) {
	addr, err := envcfg.Required(EnvHTTPAddr)
	if err != nil {
		return "", err
	}
	sec, err := envcfg.RequiredInt(EnvProfileSeconds)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s/debug/pprof/profile?seconds=%d", addr, sec), nil
}

// Stop shuts down the pprof server.
func (s *Server) Stop(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

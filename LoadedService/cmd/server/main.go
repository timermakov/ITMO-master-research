package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/itmo-vkr/dwss/LoadedService/internal/config"
	"github.com/itmo-vkr/dwss/LoadedService/internal/httpserver"
	"github.com/itmo-vkr/dwss/LoadedService/internal/workload"
	"github.com/itmo-vkr/dwss/internal/pprofserver"
	"github.com/itmo-vkr/dwss/warmkit"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}

	work := workload.New(cfg.IndexKeys)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pprofSrv, err := pprofserver.StartFromEnv(ctx, logger)
	if err != nil {
		logger.Error("pprof", slog.String("err", err.Error()))
		os.Exit(1)
	}

	var wk warmkit.Engine
	var activeReg warmkit.Registry
	if cfg.Role == "warmup" {
		reg, err := newRegistry(ctx, cfg, logger)
		if err != nil {
			logger.Error("registry", slog.String("err", err.Error()))
			os.Exit(1)
		}
		wk = warmkit.NewEngine(cfg.WarmkitConfig(), reg, logger)
		if err := wk.Start(ctx); err != nil {
			logger.Error("warmkit start", slog.String("err", err.Error()))
			os.Exit(1)
		}
		defer func() {
			shCtx, shCancel := context.WithTimeout(context.Background(), cfg.WarmupTimeout)
			defer shCancel()
			_ = wk.Stop(shCtx)
		}()
	}
	if cfg.Role == "active" {
		reg, err := registerActive(ctx, cfg, logger)
		if err != nil {
			logger.Error("register active", slog.String("err", err.Error()))
			os.Exit(1)
		}
		activeReg = reg
	}
	if activeReg != nil {
		defer func() { _ = activeReg.Close() }()
	}

	srv := httpserver.New(cfg.HTTPAddr, cfg.Role, logger, work, wk)
	if err := srv.Start(); err != nil {
		logger.Error("start server", slog.String("err", err.Error()))
		os.Exit(1)
	}
	logger.Info("service ready",
		slog.String("addr", cfg.HTTPAddr),
		slog.String("role", cfg.Role),
		slog.String("backend", string(cfg.Coordination)),
	)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shCtx, shCancel := context.WithTimeout(context.Background(), cfg.WarmupTimeout)
	defer shCancel()
	_ = srv.Stop(shCtx)
	if pprofSrv != nil {
		_ = pprofSrv.Stop(shCtx)
	}
}

func newRegistry(_ context.Context, cfg config.Settings, logger *slog.Logger) (warmkit.Registry, error) {
	switch cfg.Coordination {
	case warmkit.BackendOpenFeature:
		return warmkit.NewOFRegistry(cfg.OFConfig(), logger), nil
	default:
		return warmkit.ConnectZK(cfg.ZKEndpoints, cfg.WarmupTimeout)
	}
}

func registerActive(ctx context.Context, cfg config.Settings, logger *slog.Logger) (warmkit.Registry, error) {
	switch cfg.Coordination {
	case warmkit.BackendOpenFeature:
		regCtx, cancel := context.WithTimeout(ctx, cfg.WarmupTimeout)
		defer cancel()
		return warmkit.RegisterActiveOF(regCtx, cfg.OFConfig(), logger, cfg.ServiceName, cfg.InstanceID, cfg.HTTPAddr)
	default:
		return warmkit.RegisterActive(ctx, cfg.ZKEndpoints, cfg.WarmupTimeout, cfg.ServiceName, cfg.InstanceID, cfg.HTTPAddr)
	}
}

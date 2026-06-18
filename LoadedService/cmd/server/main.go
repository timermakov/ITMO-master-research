package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/itmo-vkr/dwss/LoadedService/internal/config"
	"github.com/itmo-vkr/dwss/LoadedService/internal/httpserver"
	"github.com/itmo-vkr/dwss/LoadedService/internal/mmapstore"
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

	sizeBytes := int64(cfg.MmapSizeMB) * 1024 * 1024
	store := mmapstore.New(cfg.MmapFile, sizeBytes)
	if err := store.EnsureFile(); err != nil {
		logger.Error("ensure file", slog.String("err", err.Error()))
		os.Exit(1)
	}
	if err := store.Map(); err != nil {
		logger.Error("map file", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = store.Unmap() }()

	work := workload.New(store, cfg.IndexKeys)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pprofSrv, err := pprofserver.StartFromEnv(ctx, logger)
	if err != nil {
		logger.Error("pprof", slog.String("err", err.Error()))
		os.Exit(1)
	}

	var wk warmkit.Engine
	var zkReg *warmkit.ZKRegistry
	if cfg.Role == "warmup" {
		reg, err := warmkit.ConnectZK(cfg.ZKEndpoints, cfg.WarmupTimeout)
		if err != nil {
			logger.Error("zk connect", slog.String("err", err.Error()))
			os.Exit(1)
		}
		zkReg = reg
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
		reg, err := warmkit.RegisterActive(ctx, cfg.ZKEndpoints, cfg.WarmupTimeout, cfg.ServiceName, cfg.InstanceID, cfg.HTTPAddr)
		if err != nil {
			logger.Error("zk register active", slog.String("err", err.Error()))
			os.Exit(1)
		}
		zkReg = reg
	}
	if zkReg != nil {
		defer func() { _ = zkReg.Close() }()
	}

	srv := httpserver.New(cfg.HTTPAddr, cfg.Role, logger, work, wk)
	if err := srv.Start(); err != nil {
		logger.Error("start server", slog.String("err", err.Error()))
		os.Exit(1)
	}
	logger.Info("service ready", slog.String("addr", cfg.HTTPAddr), slog.String("role", cfg.Role))

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

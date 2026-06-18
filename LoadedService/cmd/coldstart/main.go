package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/itmo-vkr/loadedservice/internal/httpserver"
	"github.com/itmo-vkr/loadedservice/internal/mmapstore"
)

func getenv(key, def string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	return v
}

func getenvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	// Flags with env fallbacks.
	addr := flag.String("addr", getenv("HTTP_ADDR", ":8080"), "http listen address")
	file := flag.String("file", getenv("MMAP_FILE", "data/big.bin"), "path to data file")
	sizeMB := flag.Int("size", getenvInt("MMAP_SIZE_MB", 500), "file size in MB (created if absent)")
	samples := flag.Int("samples", getenvInt("SAMPLES_DEFAULT", 1000), "default random read samples")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	sizeBytes := int64(*sizeMB) * 1024 * 1024
	store := mmapstore.New(*file, sizeBytes)
	// Create or resize to match configured size.
	if fi, err := os.Stat(*file); os.IsNotExist(err) {
		logger.Info("creating data file", slog.String("path", *file), slog.Int("size_mb", *sizeMB))
		if err := store.EnsureFile(); err != nil {
			logger.Error("ensure file", slog.String("err", err.Error()))
			os.Exit(1)
		}
	} else if err == nil && fi.Size() != sizeBytes {
		logger.Info("resizing data file", slog.String("path", *file), slog.Int64("old_bytes", fi.Size()), slog.Int64("new_bytes", sizeBytes))
		if err := store.EnsureFile(); err != nil {
			logger.Error("resize file", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}
	if err := store.Map(); err != nil {
		logger.Error("map file", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if err := store.Unmap(); err != nil {
			logger.Error("unmap", slog.String("err", err.Error()))
		}
	}()

	srv := httpserver.New(httpserver.Config{Addr: *addr, DefaultSamples: *samples}, logger, store)
	if err := srv.Start(); err != nil {
		logger.Error("start server", slog.String("err", err.Error()))
		os.Exit(1)
	}
	logger.Info("service ready", slog.String("addr", *addr), slog.String("file", *file), slog.Int("size_mb", *sizeMB))

	// Graceful shutdown on SIGINT/SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutdown requested")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
	}
}

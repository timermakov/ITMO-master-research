package main

import (
	"context"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/itmo-vkr/dwss/internal/envcfg"
	"github.com/itmo-vkr/dwss/internal/pprofserver"
	"github.com/itmo-vkr/dwss/warmkit"
)

const (
	envHTTPAddr           = "DWSS_PROXY_HTTP_ADDR"
	envCoordination       = "DWSS_COORDINATION"
	envZKEndpoints        = "DWSS_PROXY_ZK_ENDPOINTS"
	envServiceName        = "DWSS_PROXY_SERVICE_NAME"
	envShutdownSec        = "DWSS_PROXY_SHUTDOWN_SEC"
	envFlagdOFREPURL      = "DWSS_FLAGD_OFREP_URL"
	envFlagdPollMS        = "DWSS_FLAGD_POLL_MS"
	envFlagdHTTPTimeoutMS = "DWSS_FLAGD_HTTP_TIMEOUT_MS"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr, err := envcfg.Required(envHTTPAddr)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	backendRaw, err := envcfg.Required(envCoordination)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	backend, err := warmkit.ParseBackend(backendRaw)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	service, err := envcfg.Required(envServiceName)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	shutdownSec, err := envcfg.RequiredInt(envShutdownSec)
	if err != nil {
		logger.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pprofSrv, err := pprofserver.StartFromEnv(ctx, logger)
	if err != nil {
		logger.Error("pprof", slog.String("err", err.Error()))
		os.Exit(1)
	}

	views, closer, err := coordinationViews(ctx, logger, backend, service, time.Duration(shutdownSec)*time.Second)
	if err != nil {
		logger.Error("coordination", slog.String("err", err.Error()))
		os.Exit(1)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	router := newProxyRouter(logger)
	go router.consume(ctx, views)

	mux := http.NewServeMux()
	mux.Handle("/", router)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", slog.String("err", err.Error()))
		}
	}()
	logger.Info("mirror-proxy listening", slog.String("addr", addr), slog.String("backend", string(backend)))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shCtx, shCancel := context.WithTimeout(context.Background(), time.Duration(shutdownSec)*time.Second)
	defer shCancel()
	_ = srv.Shutdown(shCtx)
	if pprofSrv != nil {
		_ = pprofSrv.Stop(shCtx)
	}
}

func coordinationViews(ctx context.Context, log *slog.Logger, backend warmkit.Backend, service string, timeout time.Duration) (<-chan warmkit.CoordinationView, interface{ Close() error }, error) {
	switch backend {
	case warmkit.BackendZooKeeper:
		endpoints, err := envcfg.RequiredCSV(envZKEndpoints)
		if err != nil {
			return nil, nil, err
		}
		zkReg, err := warmkit.ConnectZK(endpoints, timeout)
		if err != nil {
			return nil, nil, err
		}
		ch, err := zkReg.WatchView(ctx, service)
		if err != nil {
			_ = zkReg.Close()
			return nil, nil, err
		}
		return ch, zkReg, nil
	case warmkit.BackendOpenFeature:
		ofrepURL, err := envcfg.Required(envFlagdOFREPURL)
		if err != nil {
			return nil, nil, err
		}
		pollMS, err := envcfg.RequiredInt(envFlagdPollMS)
		if err != nil {
			return nil, nil, err
		}
		httpTimeoutMS, err := envcfg.RequiredInt(envFlagdHTTPTimeoutMS)
		if err != nil {
			return nil, nil, err
		}
		store := warmkit.NewOFViewStore(warmkit.OFConfig{
			OFREPURL:     ofrepURL,
			PollInterval: time.Duration(pollMS) * time.Millisecond,
			HTTPTimeout:  time.Duration(httpTimeoutMS) * time.Millisecond,
		}, log)
		ch, err := store.WatchView(ctx, service)
		if err != nil {
			_ = store.Close()
			return nil, nil, err
		}
		return ch, store, nil
	default:
		return nil, nil, errUnknownBackend(backend)
	}
}

func errUnknownBackend(backend warmkit.Backend) error {
	return &backendError{backend: backend}
}

type backendError struct {
	backend warmkit.Backend
}

func (e *backendError) Error() string {
	return "unknown coordination backend " + string(e.backend)
}

type proxyRouter struct {
	log  *slog.Logger
	mu   sync.RWMutex
	cfg  warmkit.MirrorConfig
	inst map[string]string
}

func newProxyRouter(log *slog.Logger) *proxyRouter {
	return &proxyRouter{
		log:  log,
		inst: make(map[string]string),
	}
}

func (p *proxyRouter) consume(ctx context.Context, views <-chan warmkit.CoordinationView) {
	for {
		select {
		case <-ctx.Done():
			return
		case view, ok := <-views:
			if !ok {
				return
			}
			inst := make(map[string]string, len(view.Instances))
			for id, rec := range view.Instances {
				inst[id] = rec.Addr
			}
			p.mu.Lock()
			p.cfg = view.Mirror
			p.inst = inst
			p.mu.Unlock()
			p.log.Info("coordination view",
				slog.Bool("mirrorEnabled", view.Mirror.Enabled),
				slog.Float64("ratio", view.Mirror.Ratio),
				slog.Int("instances", len(inst)),
			)
		}
	}
}

func (p *proxyRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	cfg := p.cfg
	inst := p.inst
	p.mu.RUnlock()

	activeAddr, ok := inst[cfg.ActiveInstanceID]
	if !ok || activeAddr == "" {
		http.Error(w, "active instance unavailable", http.StatusServiceUnavailable)
		return
	}
	target, err := url.Parse("http://" + activeAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)

	if r.Method == http.MethodGet && cfg.Enabled && cfg.Ratio > 0 {
		if rand.Float64() < cfg.Ratio {
			warmAddr, ok := inst[cfg.TargetInstanceID]
			if ok && warmAddr != "" {
				go p.mirrorRequest(r, warmAddr)
			}
		}
	}
}

func (p *proxyRouter) mirrorRequest(r *http.Request, addr string) {
	u, err := url.Parse("http://" + addr + r.URL.RequestURI())
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return
	}
	req.Header = r.Header.Clone()
	req.Header.Set(warmkit.HeaderShadow, warmkit.ShadowValue)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

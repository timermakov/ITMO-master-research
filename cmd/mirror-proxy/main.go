package main

import (
	"context"
	"encoding/json"
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

	"github.com/go-zookeeper/zk"
	"github.com/itmo-vkr/dwss/internal/envcfg"
	"github.com/itmo-vkr/dwss/internal/pprofserver"
	"github.com/itmo-vkr/dwss/warmkit"
)

const (
	envHTTPAddr    = "DWSS_PROXY_HTTP_ADDR"
	envZKEndpoints = "DWSS_PROXY_ZK_ENDPOINTS"
	envServiceName = "DWSS_PROXY_SERVICE_NAME"
	envShutdownSec = "DWSS_PROXY_SHUTDOWN_SEC"
)

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

	conn, _, err := zk.Connect(endpoints, time.Duration(shutdownSec)*time.Second)
	if err != nil {
		logger.Error("zk", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer conn.Close()

	router := newProxyRouter(logger, conn, service)
	go router.watchMirror(ctx)
	go router.watchInstances(ctx)

	mux := http.NewServeMux()
	mux.Handle("/", router)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", slog.String("err", err.Error()))
		}
	}()
	logger.Info("mirror-proxy listening", slog.String("addr", addr))

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

type proxyRouter struct {
	log     *slog.Logger
	conn    *zk.Conn
	service string
	mu      sync.RWMutex
	cfg     warmkit.MirrorConfig
	inst    map[string]string
}

func newProxyRouter(log *slog.Logger, conn *zk.Conn, service string) *proxyRouter {
	return &proxyRouter{
		log:     log,
		conn:    conn,
		service: service,
		inst:    make(map[string]string),
	}
}

func (p *proxyRouter) watchMirror(ctx context.Context) {
	path := "/config/" + p.service + "/mirror"
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		cfg, err := warmkit.ReadMirrorConfig(p.conn, p.service)
		if err == nil {
			p.mu.Lock()
			p.cfg = cfg
			p.mu.Unlock()
		}
		_, _, events, err := p.conn.GetW(path)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		select {
		case <-events:
		case <-ctx.Done():
			return
		}
	}
}

func (p *proxyRouter) watchInstances(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		children, _, ch, err := p.conn.ChildrenW("/services/" + p.service + "/instances")
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		p.refreshInstances(children)
		select {
		case ev := <-ch:
			if ev.Type == zk.EventNodeChildrenChanged {
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *proxyRouter) refreshInstances(children []string) {
	m := make(map[string]string)
	base := "/services/" + p.service + "/instances/"
	for _, id := range children {
		b, _, err := p.conn.Get(base + id)
		if err != nil {
			continue
		}
		var rec warmkit.InstanceRecord
		if json.Unmarshal(b, &rec) == nil {
			m[id] = rec.Addr
		}
	}
	p.mu.Lock()
	p.inst = m
	p.mu.Unlock()
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

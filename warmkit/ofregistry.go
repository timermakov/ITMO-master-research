package warmkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"
)

// OFConfig is the OpenFeature/flagd client configuration.
type OFConfig struct {
	OFREPURL       string
	CoordinatorURL string
	PollInterval   time.Duration
	HTTPTimeout    time.Duration
}

// OFViewStore watches coordination flags through flagd OFREP.
type OFViewStore struct {
	log  *slog.Logger
	of   *ofrepClient
	poll time.Duration
}

// NewOFViewStore evaluates flags via OpenFeature OFREP on flagd.
func NewOFViewStore(cfg OFConfig, log *slog.Logger) *OFViewStore {
	if log == nil {
		log = slog.Default()
	}
	return &OFViewStore{
		log:  log,
		of:   newOFREPClient(cfg.OFREPURL, cfg.HTTPTimeout),
		poll: cfg.PollInterval,
	}
}

// WatchView polls OFREP and emits a snapshot when mirror or instances change.
func (s *OFViewStore) WatchView(ctx context.Context, service string) (<-chan CoordinationView, error) {
	ch := make(chan CoordinationView, 1)
	go func() {
		defer close(ch)
		var last CoordinationView
		have := false
		ticker := time.NewTicker(s.poll)
		defer ticker.Stop()
		emit := func() {
			view, err := s.of.view(ctx, service)
			if err != nil {
				s.log.Debug("ofrep view", slog.String("err", err.Error()))
				return
			}
			if have && reflect.DeepEqual(last, view) {
				return
			}
			last = view
			have = true
			select {
			case ch <- cloneView(view):
			case <-ctx.Done():
			}
		}
		emit()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				emit()
			}
		}
	}()
	return ch, nil
}

func (s *OFViewStore) Close() error { return nil }

// OFRegistry is a warmkit Registry: writes go to coordinator HTTP, reads from flagd.
type OFRegistry struct {
	*OFViewStore
	coordURL   string
	http       *http.Client
	instanceID string
}

// NewOFRegistry creates an OpenFeature-backed registry.
func NewOFRegistry(cfg OFConfig, log *slog.Logger) *OFRegistry {
	return &OFRegistry{
		OFViewStore: NewOFViewStore(cfg, log),
		coordURL:    strings.TrimRight(cfg.CoordinatorURL, "/"),
		http:        &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

// RegisterInstance publishes the instance through coordinator (sole flag writer).
func (r *OFRegistry) RegisterInstance(ctx context.Context, service, instanceID string, rec InstanceRecord) error {
	_ = service
	r.instanceID = instanceID
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return r.doJSON(ctx, http.MethodPut, InstancePath(instanceID), b, http.StatusOK, true)
}

// UpdateInstanceState updates the instance flag via coordinator.
func (r *OFRegistry) UpdateInstanceState(ctx context.Context, service, instanceID string, state State) error {
	_ = service
	b, err := json.Marshal(map[string]string{"state": string(state)})
	if err != nil {
		return err
	}
	return r.doJSON(ctx, http.MethodPut, InstanceStatePath(instanceID), b, http.StatusOK, false)
}

// WatchMirror maps OFREP snapshots onto the mirror-config channel used by Engine.
func (r *OFRegistry) WatchMirror(ctx context.Context, service string) (<-chan MirrorConfig, error) {
	views, err := r.WatchView(ctx, service)
	if err != nil {
		return nil, err
	}
	ch := make(chan MirrorConfig, 1)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case view, ok := <-views:
				if !ok {
					return
				}
				select {
				case ch <- view.Mirror:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

// Close deregisters the instance so flagd stops advertising it.
func (r *OFRegistry) Close() error {
	if r.instanceID == "" || r.coordURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.http.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, r.coordURL+InstancePath(r.instanceID), nil)
	if err != nil {
		return err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deregister status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (r *OFRegistry) doJSON(ctx context.Context, method, path string, body []byte, want int, retry bool) error {
	attempt := func() error {
		req, err := http.NewRequestWithContext(ctx, method, r.coordURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := r.http.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == want {
			return nil
		}
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if !retry {
		return attempt()
	}
	var last error
	for {
		last = attempt()
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s %s: %w", method, path, last)
		case <-time.After(r.poll):
		}
	}
}

func cloneView(v CoordinationView) CoordinationView {
	inst := make(map[string]InstanceRecord, len(v.Instances))
	for k, rec := range v.Instances {
		inst[k] = rec
	}
	return CoordinationView{Mirror: v.Mirror, Instances: inst}
}

// RegisterActiveOF registers a production instance through coordinator HTTP.
func RegisterActiveOF(ctx context.Context, cfg OFConfig, log *slog.Logger, service, instanceID, addr string) (*OFRegistry, error) {
	reg := NewOFRegistry(cfg, log)
	rec := InstanceRecord{
		Addr:      addr,
		State:     StateActive,
		Role:      "active",
		StartedAt: time.Now().UnixMilli(),
	}
	if err := reg.RegisterInstance(ctx, service, instanceID, rec); err != nil {
		_ = reg.Close()
		return nil, err
	}
	return reg, nil
}

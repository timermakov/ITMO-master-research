package warmkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
)

// Registry abstracts instance and mirror config storage.
type Registry interface {
	RegisterInstance(ctx context.Context, service, instanceID string, rec InstanceRecord) error
	UpdateInstanceState(ctx context.Context, service, instanceID string, state State) error
	WatchMirror(ctx context.Context, service string) (<-chan MirrorConfig, error)
	Close() error
}

// ZKRegistry implements Registry with ZooKeeper.
type ZKRegistry struct {
	conn *zk.Conn
}

func ConnectZK(endpoints []string, timeout time.Duration) (*ZKRegistry, error) {
	conn, _, err := zk.Connect(endpoints, timeout)
	if err != nil {
		return nil, err
	}
	return &ZKRegistry{conn: conn}, nil
}

func (z *ZKRegistry) ensurePath(path string) error {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	cur := ""
	for _, p := range parts {
		cur += "/" + p
		exists, _, err := z.conn.Exists(cur)
		if err != nil {
			return err
		}
		if !exists {
			_, err = z.conn.Create(cur, nil, 0, zk.WorldACL(zk.PermAll))
			if err != nil && err != zk.ErrNodeExists {
				return err
			}
		}
	}
	return nil
}

func (z *ZKRegistry) RegisterInstance(ctx context.Context, service, instanceID string, rec InstanceRecord) error {
	base := fmt.Sprintf("/services/%s/instances", service)
	if err := z.ensurePath(base); err != nil {
		return err
	}
	path := base + "/" + instanceID
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = z.conn.Create(path, b, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
	if err == zk.ErrNodeExists {
		_, err = z.conn.Set(path, b, -1)
	}
	return err
}

func (z *ZKRegistry) UpdateInstanceState(ctx context.Context, service, instanceID string, state State) error {
	path := fmt.Sprintf("/services/%s/instances/%s", service, instanceID)
	b, stat, err := z.conn.Get(path)
	if err != nil {
		return err
	}
	var rec InstanceRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return err
	}
	rec.State = state
	nb, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = z.conn.Set(path, nb, stat.Version)
	return err
}

func (z *ZKRegistry) WatchMirror(ctx context.Context, service string) (<-chan MirrorConfig, error) {
	ch := make(chan MirrorConfig, 1)
	path := fmt.Sprintf("/config/%s/mirror", service)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			b, _, events, err := z.conn.GetW(path)
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			var cfg MirrorConfig
			if len(b) > 0 {
				_ = json.Unmarshal(b, &cfg)
			}
			select {
			case ch <- cfg:
			case <-ctx.Done():
				return
			}
			select {
			case <-events:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// RegisterActive registers a production instance in ZK and returns a handle that must
// remain open for the ephemeral node to stay alive.
func RegisterActive(ctx context.Context, endpoints []string, sessionTimeout time.Duration, service, instanceID, addr string) (*ZKRegistry, error) {
	reg, err := ConnectZK(endpoints, sessionTimeout)
	if err != nil {
		return nil, err
	}
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

func (z *ZKRegistry) Close() error {
	z.conn.Close()
	return nil
}

// WriteMirrorConfig writes mirror config (coordinator only).
func WriteMirrorConfig(conn *zk.Conn, service string, cfg MirrorConfig) error {
	path := fmt.Sprintf("/config/%s/mirror", service)
	zr := &ZKRegistry{conn: conn}
	if err := zr.ensurePath("/config/" + service); err != nil {
		return err
	}
	cfg.UpdatedAtUnixMilli = time.Now().UnixMilli()
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	exists, _, err := conn.Exists(path)
	if err != nil {
		return err
	}
	if !exists {
		_, err = conn.Create(path, b, 0, zk.WorldACL(zk.PermAll))
		return err
	}
	_, err = conn.Set(path, b, -1)
	return err
}

// ReadMirrorConfig reads current mirror config.
func ReadMirrorConfig(conn *zk.Conn, service string) (MirrorConfig, error) {
	path := fmt.Sprintf("/config/%s/mirror", service)
	b, _, err := conn.Get(path)
	if err != nil {
		return MirrorConfig{}, err
	}
	var cfg MirrorConfig
	if len(b) > 0 {
		err = json.Unmarshal(b, &cfg)
	}
	return cfg, err
}

// MemoryRegistry is an in-memory Registry for tests.
type MemoryRegistry struct {
	mu        sync.RWMutex
	instances map[string]InstanceRecord
	mirror    MirrorConfig
	watchCh   chan MirrorConfig
}

func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		instances: make(map[string]InstanceRecord),
		watchCh:   make(chan MirrorConfig, 8),
	}
}

func (m *MemoryRegistry) key(service, id string) string {
	return service + "/" + id
}

func (m *MemoryRegistry) RegisterInstance(ctx context.Context, service, instanceID string, rec InstanceRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instances[m.key(service, instanceID)] = rec
	return nil
}

func (m *MemoryRegistry) UpdateInstanceState(ctx context.Context, service, instanceID string, state State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(service, instanceID)
	rec, ok := m.instances[k]
	if !ok {
		return fmt.Errorf("instance not found")
	}
	rec.State = state
	m.instances[k] = rec
	return nil
}

func (m *MemoryRegistry) WatchMirror(ctx context.Context, service string) (<-chan MirrorConfig, error) {
	out := make(chan MirrorConfig, 8)
	go func() {
		defer close(out)
		m.mu.RLock()
		cfg := m.mirror
		m.mu.RUnlock()
		select {
		case out <- cfg:
		case <-ctx.Done():
			return
		}
		for {
			select {
			case cfg, ok := <-m.watchCh:
				if !ok {
					return
				}
				select {
				case out <- cfg:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (m *MemoryRegistry) SetMirror(cfg MirrorConfig) {
	m.mu.Lock()
	m.mirror = cfg
	m.mu.Unlock()
	select {
	case m.watchCh <- cfg:
	default:
	}
}

func (m *MemoryRegistry) Close() error { return nil }

// IsShadowRequest reports whether r is a shadow warmup request.
func IsShadowRequest(r *http.Request) bool {
	return r.Header.Get(HeaderShadow) == ShadowValue
}

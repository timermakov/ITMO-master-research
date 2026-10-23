package warmkit

import (
	"fmt"
	"sync"
	"time"
)

// FlagSource is the OpenFeature control-plane store. warmup-coordinator is the
// only writer; flagd evaluates the document via GET /v1/openfeature/flags.
type FlagSource struct {
	mu        sync.RWMutex
	service   string
	mirror    MirrorConfig
	instances map[string]InstanceRecord
}

// NewFlagSource creates an empty coordination store for service.
func NewFlagSource(service string) *FlagSource {
	return &FlagSource{
		service:   service,
		instances: make(map[string]InstanceRecord),
	}
}

// WriteMirror stores mirror config (coordinator-only).
func (s *FlagSource) WriteMirror(cfg MirrorConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg.UpdatedAtUnixMilli = time.Now().UnixMilli()
	s.mirror = cfg
	return nil
}

// ReadMirror returns the current mirror config.
func (s *FlagSource) ReadMirror() MirrorConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mirror
}

// PutInstance registers or replaces an instance record.
func (s *FlagSource) PutInstance(instanceID string, rec InstanceRecord) error {
	if instanceID == "" {
		return fmt.Errorf("instance id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[instanceID] = rec
	return nil
}

// UpdateInstanceState updates one instance FSM state.
func (s *FlagSource) UpdateInstanceState(instanceID string, state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.instances[instanceID]
	if !ok {
		return fmt.Errorf("instance %q not found", instanceID)
	}
	rec.State = state
	s.instances[instanceID] = rec
	return nil
}

// DeleteInstance removes an instance from the registry.
func (s *FlagSource) DeleteInstance(instanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.instances, instanceID)
}

// GetInstance returns a registered instance.
func (s *FlagSource) GetInstance(instanceID string) (InstanceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.instances[instanceID]
	if !ok {
		return InstanceRecord{}, fmt.Errorf("instance %q not found", instanceID)
	}
	return rec, nil
}

// Instances returns a copy of the instance map.
func (s *FlagSource) Instances() map[string]InstanceRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]InstanceRecord, len(s.instances))
	for k, v := range s.instances {
		out[k] = v
	}
	return out
}

// Document returns the flagd-compatible OpenFeature flag set.
func (s *FlagSource) Document() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inst := make(map[string]InstanceRecord, len(s.instances))
	for k, v := range s.instances {
		inst[k] = v
	}
	return BuildFlagdDocument(s.service, s.mirror, inst)
}

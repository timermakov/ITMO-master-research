package warmkit

import (
	"sync"
	"time"
)

// FSM manages warmup lifecycle transitions.
type FSM struct {
	mu    sync.RWMutex
	state State
}

func NewFSM() *FSM {
	return &FSM{state: StateStarting}
}

func (f *FSM) State() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state
}

func (f *FSM) Transition(next State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !validTransition(f.state, next) {
		return ErrInvalidTransition{From: f.state, To: next}
	}
	f.state = next
	return nil
}

func (f *FSM) Force(next State) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = next
}

func validTransition(from, to State) bool {
	switch from {
	case StateStarting:
		return to == StateRegistered || to == StateFailed
	case StateRegistered:
		return to == StateWarming || to == StateFailed
	case StateWarming:
		return to == StateReady || to == StateFailed
	case StateReady:
		return to == StateActive || to == StateFailed
	case StateActive, StateFailed:
		return false
	default:
		return false
	}
}

// ErrInvalidTransition is returned when FSM cannot move to the target state.
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return "invalid transition: " + string(e.From) + " -> " + string(e.To)
}

// ShadowMetrics tracks shadow request statistics.
type ShadowMetrics struct {
	mu        sync.Mutex
	count     int64
	latencies []time.Duration
	maxKeep   int
}

func NewShadowMetrics(maxKeep int) *ShadowMetrics {
	return &ShadowMetrics{maxKeep: maxKeep}
}

func (m *ShadowMetrics) Record(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	m.latencies = append(m.latencies, d)
	if len(m.latencies) > m.maxKeep {
		m.latencies = m.latencies[len(m.latencies)-m.maxKeep:]
	}
}

func (m *ShadowMetrics) Count() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func (m *ShadowMetrics) P95Ns() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.latencies) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), m.latencies...)
	p95 := percentileDuration(cp, 0.95)
	return p95.Nanoseconds()
}

func percentileDuration(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

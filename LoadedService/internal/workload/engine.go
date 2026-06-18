package workload

import (
	"math/rand"
	"sync"

	"github.com/itmo-vkr/dwss/LoadedService/internal/mmapstore"
	"github.com/itmo-vkr/dwss/warmkit"
)

// Engine combines mmap reads and lazy in-memory index.
type Engine struct {
	store      *mmapstore.Store
	indexKeys  int
	mu         sync.Mutex
	index      map[int]int
	indexBuilt bool
	mmapCold   bool
	appCold    bool
}

// New creates a workload engine.
func New(store *mmapstore.Store, indexKeys int) *Engine {
	return &Engine{
		store:     store,
		indexKeys: indexKeys,
		mmapCold:  true,
		appCold:   true,
	}
}

// State exposes cold flags for /state.
type State struct {
	MmapCold bool `json:"mmapCold"`
	AppCold  bool `json:"appCold"`
}

func (e *Engine) SnapshotState() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return State{MmapCold: e.mmapCold, AppCold: e.appCold}
}

func (e *Engine) ensureIndex() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.indexBuilt {
		return
	}
	e.index = make(map[int]int, e.indexKeys)
	for i := 0; i < e.indexKeys; i++ {
		e.index[i] = i * 31
	}
	e.indexBuilt = true
	e.appCold = false
}

// RunProfile executes the same access pattern for /work and /warmup.
func (e *Engine) RunProfile(p warmkit.WorkloadProfile) int64 {
	e.ensureIndex()
	indices := e.store.RandomPageIndices(p.Samples, p.Seed)
	d := e.store.MeasureReadDurationAt(indices)
	e.mu.Lock()
	e.mmapCold = false
	e.mu.Unlock()
	r := rand.New(rand.NewSource(p.Seed))
	for i := 0; i < p.Samples; i++ {
		k := r.Intn(e.indexKeys)
		_ = e.index[k]
	}
	return d.Nanoseconds()
}

// WarmProfile touches the same pages/keys as RunProfile without measuring separately.
func (e *Engine) WarmProfile(p warmkit.WorkloadProfile) {
	_ = e.RunProfile(p)
}

// ResetCold drops lazy index and remaps mmap to simulate a cold instance.
func (e *Engine) ResetCold() error {
	e.mu.Lock()
	e.index = nil
	e.indexBuilt = false
	e.mmapCold = true
	e.appCold = true
	e.mu.Unlock()
	return e.store.Remap()
}

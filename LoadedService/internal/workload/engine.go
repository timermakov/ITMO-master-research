package workload

import (
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

// Engine runs index-build workload: lazy map[int]int build plus lookups.
type Engine struct {
	indexKeys  int
	mu         sync.Mutex
	index      map[int]int
	indexBuilt bool
	indexCold  bool
}

// New creates a workload engine.
func New(indexKeys int) *Engine {
	return &Engine{
		indexKeys: indexKeys,
		indexCold: true,
	}
}

// State exposes cold flag for /state.
type State struct {
	IndexCold bool `json:"indexCold"`
}

func (e *Engine) SnapshotState() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return State{IndexCold: e.indexCold}
}

// RunProfile builds the index if needed, performs lookups, returns total duration.
func (e *Engine) RunProfile(p warmkit.WorkloadProfile) int64 {
	start := time.Now()

	e.mu.Lock()
	if !e.indexBuilt {
		e.index = make(map[int]int, e.indexKeys)
		for i := 0; i < e.indexKeys; i++ {
			e.index[i] = i
		}
		e.indexBuilt = true
		e.indexCold = false
	}
	index := e.index
	e.mu.Unlock()

	r := rand.New(rand.NewSource(p.Seed))
	var sink int
	for i := 0; i < p.Samples; i++ {
		k := r.Intn(e.indexKeys)
		sink += index[k]
	}
	runtime.KeepAlive(sink)

	return time.Since(start).Nanoseconds()
}

// WarmProfile executes the same path as RunProfile.
func (e *Engine) WarmProfile(p warmkit.WorkloadProfile) {
	_ = e.RunProfile(p)
}

// ResetCold drops the in-memory index to simulate a cold instance.
func (e *Engine) ResetCold() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.index = nil
	e.indexBuilt = false
	e.indexCold = true
	return nil
}

package babymonitor

import "sync"

// Results is a thread-safe cache of the latest awake/asleep determination.
type Results struct {
	mu      sync.Mutex
	isAwake bool
}

func NewResults() *Results {
	return &Results{}
}

func (r *Results) Store(isAwake bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.isAwake = isAwake
}

func (r *Results) Load() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isAwake
}

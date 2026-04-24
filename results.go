package babymonitor

import "sync"

// Results is a thread-safe cache of the latest awake/asleep determination and raw sensor values.
type Results struct {
	mu               sync.Mutex
	isAwake          bool
	eyesDetected     bool
	motionConfidence float64
	soundDetected    bool
}

func NewResults() *Results {
	return &Results{}
}

func (r *Results) Store(isAwake, eyesDetected bool, motionConfidence float64, soundDetected bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.isAwake = isAwake
	r.eyesDetected = eyesDetected
	r.motionConfidence = motionConfidence
	r.soundDetected = soundDetected
}

func (r *Results) Load() (isAwake, eyesDetected bool, motionConfidence float64, soundDetected bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isAwake, r.eyesDetected, r.motionConfidence, r.soundDetected
}

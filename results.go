package babymonitor

import (
	"image"
	"sync"

	"go.viam.com/rdk/vision/classification"
)

// Results is a thread-safe cache of the latest frame and its classifications.
type Results struct {
	mu              sync.Mutex
	latestImg       image.Image
	classifications classification.Classifications
}

func NewResults() *Results {
	return &Results{}
}

func (r *Results) Store(img image.Image, cls classification.Classifications) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latestImg = img
	r.classifications = cls
}

func (r *Results) Load() (image.Image, classification.Classifications) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latestImg, r.classifications
}

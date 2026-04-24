package babymonitor

import (
	"context"
	"strings"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/services/vision"
)

// runPipeline reads from all three input resources and returns whether the baby is awake.
func runPipeline(
	ctx context.Context,
	camName string,
	eyeDetector, motionDetector vision.Service,
	audioSensor sensor.Sensor,
	minEyeConf float64,
	logger logging.Logger,
) (bool, error) {
	// Step 1: Eye signal — any eyes detected above threshold means the baby is visible.
	eyesDetected := false
	eyeConfidence := 0.0
	detections, detErr := eyeDetector.DetectionsFromCamera(ctx, camName, nil)
	if detErr != nil {
		logger.Warnw("eye detector error", "error", detErr)
	} else {
		for _, det := range detections {
			if !strings.EqualFold(det.Label(), "eyes") {
				continue
			}
			score := float64(det.Score())
			if score >= minEyeConf && score > eyeConfidence {
				eyeConfidence = score
				eyesDetected = true
			}
		}
	}

	// Step 2: Motion signal.
	motionConfidence := 0.0
	motionCls, motionErr := motionDetector.ClassificationsFromCamera(ctx, camName, 1, nil)
	if motionErr != nil {
		logger.Warnw("motion detector error", "error", motionErr)
	} else {
		for _, c := range motionCls {
			if strings.EqualFold(c.Label(), "motion") {
				motionConfidence = float64(c.Score())
				break
			}
		}
	}

	// Step 3: Audio signal — expects a "sound_level" key in [0,1] from the sensor.
	audioLevel := 0.0
	audioReadings, audioErr := audioSensor.Readings(ctx, nil)
	if audioErr != nil {
		logger.Warnw("audio sensor error", "error", audioErr)
	} else {
		if v, ok := audioReadings["sound_level"]; ok {
			if f, ok := v.(float64); ok {
				audioLevel = f
			}
		}
	}

	// Step 4: Fuse all three signals.
	return fuseSignals(eyesDetected, eyeConfidence, motionConfidence, audioLevel), nil
}

// fuseSignals combines eye, motion, and audio signals into an awake confidence in [0,1].
// Parameters:
//   - eyesDetected:     whether any eyes were found above minEyeConf
//   - eyeConfidence:    detection confidence of the best eye bounding box (0 if none)
//   - motionConfidence: fraction of pixels that moved (from viam/motion-detector)
//   - audioLevel:       normalised sound level in [0,1] (from audio-sensor)
//
// Eyes and audio are hard awake signals — either alone is sufficient.
// Motion is intentionally left as a weak signal pending real-world tuning.
func fuseSignals(eyesDetected bool, eyeConfidence, motionConfidence, audioLevel float64) bool {
	if eyesDetected {
		return true
	}
	if audioLevel > 0.2 {
		return true
	}
	return motionConfidence+audioLevel > 0.2
}

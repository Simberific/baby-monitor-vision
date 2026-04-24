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

	// Step 3: Audio signal — expects a "sound_detected" bool key from the sensor.
	audioDetected := false
	audioReadings, audioErr := audioSensor.Readings(ctx, nil)
	if audioErr != nil {
		logger.Warnw("audio sensor error", "error", audioErr)
	} else {
		if v, ok := audioReadings["sound_detected"]; ok {
			if b, ok := v.(bool); ok {
				audioDetected = b
			}
		}
	}

	// Step 4: Fuse all three signals.
	return fuseSignals(eyesDetected, eyeConfidence, motionConfidence, audioDetected), nil
}

// fuseSignals combines eye, motion, and audio signals into an awake determination.
// Eyes and audio are hard awake signals — either alone is sufficient.
// Motion alone triggers awake above a confidence threshold.
func fuseSignals(eyesDetected bool, eyeConfidence, motionConfidence float64, audioDetected bool) bool {
	if eyesDetected {
		return true
	}
	if audioDetected {
		return true
	}
	return motionConfidence > 0.1
}

package babymonitor

import (
	"context"
	"image"
	"image/draw"
	"strings"

	"github.com/pkg/errors"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/services/vision"
	"go.viam.com/rdk/vision/classification"
)

// cropImage returns a copy of img restricted to rect, using SubImage when available.
// Copied verbatim from fish-predictor/bbox_helpers.go.
func cropImage(img image.Image, rect image.Rectangle) image.Image {
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if sub, ok := img.(subImager); ok {
		return sub.SubImage(rect)
	}
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	return dst
}

// runPipeline executes the full detection pipeline for a single frame and returns
// a single "awake" classification whose confidence encodes awake probability.
func runPipeline(
	ctx context.Context,
	img image.Image,
	camName string,
	eyeDetector, eyeClassifier, motionDetector vision.Service,
	minEyeConf float64,
	logger logging.Logger,
) (classification.Classifications, error) {
	// Step 1: Eye detection — find the highest-confidence "eyes" bounding box.
	detections, err := eyeDetector.Detections(ctx, img, nil)
	if err != nil {
		return nil, errors.Wrap(err, "eye detection failed")
	}

	eyesDetected := false
	eyeConfidence := 0.0
	var bestEyeBB *image.Rectangle

	for _, det := range detections {
		if !strings.EqualFold(det.Label(), "eyes") {
			continue
		}
		score := float64(det.Score())
		if score < minEyeConf {
			continue
		}
		if score > eyeConfidence {
			eyeConfidence = score
			bb := det.BoundingBox()
			bestEyeBB = bb
			eyesDetected = true
		}
	}

	// Step 2+3: Crop to eye bbox, classify open/closed.
	eyesOpen := false
	if eyesDetected && bestEyeBB != nil {
		// Clamp to image bounds before cropping.
		clampedBB := bestEyeBB.Intersect(img.Bounds())
		if !clampedBB.Empty() {
			croppedImg := cropImage(img, clampedBB)
			cls, classifyErr := eyeClassifier.Classifications(ctx, croppedImg, 5, nil)
			if classifyErr != nil {
				logger.Warnw("eye classifier error", "error", classifyErr)
			} else {
				for _, c := range cls {
					if strings.EqualFold(c.Label(), "open") {
						eyesOpen = true
						break
					}
				}
			}
		}
	}

	// Step 4: Motion signal — fraction of pixels that moved.
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

	// Step 5: Fuse all signals into a single confidence score.
	awakeConf := fuseSignals(eyesDetected, eyesOpen, eyeConfidence, motionConfidence)

	// Step 6: Always emit label "awake"; confidence encodes the probability.
	// Consumers apply their own threshold to determine awake/asleep state.
	return classification.Classifications{
		classification.NewClassification(awakeConf, "awake"),
	}, nil
}

// fuseSignals combines three signals into a single awake confidence score in [0,1].
// Parameters:
//   - eyesDetected:     whether any eyes were found with sufficient confidence
//   - eyesOpen:         whether the detected eyes are classified as open
//   - eyeConfidence:    confidence of the eye detection (0 if none detected)
//   - motionConfidence: fraction of pixels that moved (from viam/motion-detector)
//
// TODO: Refine fusion logic. Consider trade-offs:
//   - How much should motion alone contribute when eyes aren't visible?
//   - Should eyes-closed + high motion → still count as awake?
//   - What weight ratio between eye state and motion signal feels right?
func fuseSignals(eyesDetected, eyesOpen bool, eyeConfidence, motionConfidence float64) float64 {
	if !eyesDetected {
		return motionConfidence
	}
	if eyesOpen {
		return eyeConfidence*0.9 + motionConfidence*0.1
	}
	return motionConfidence * 0.5
}

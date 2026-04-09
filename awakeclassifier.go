package babymonitor

import (
	"context"
	"image"
	"strings"

	"github.com/pkg/errors"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	vis "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/classification"
	objdet "go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/rdk/vision/viscapture"
	viamutils "go.viam.com/utils"
)

var (
	AwakeClassifierModel = resource.NewModel("simone-kalmakis", "baby-monitor", "awake-classifier")
	errUnimplemented     = errors.New("unimplemented")
)

const defaultMinEyeConfidence = 0.5

func init() {
	resource.RegisterService(vision.API, AwakeClassifierModel,
		resource.Registration[vision.Service, *Config]{
			Constructor: NewAwakeClassifier,
		},
	)
}

// Config holds the configuration for the awake-classifier vision service.
type Config struct {
	CameraName         string  `json:"camera_name"`
	EyeDetectorName    string  `json:"eye_detector_name"`
	EyeClassifierName  string  `json:"eye_classifier_name"`
	MotionDetectorName string  `json:"motion_detector_name"`
	MinEyeConfidence   float64 `json:"min_eye_confidence"`
}

// Validate returns all four named services as hard dependencies so viam-server
// starts them before this module.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.CameraName == "" {
		return nil, nil, errors.New("camera_name is required")
	}
	if cfg.EyeDetectorName == "" {
		return nil, nil, errors.New("eye_detector_name is required")
	}
	if cfg.EyeClassifierName == "" {
		return nil, nil, errors.New("eye_classifier_name is required")
	}
	if cfg.MotionDetectorName == "" {
		return nil, nil, errors.New("motion_detector_name is required")
	}
	return []string{
		cfg.CameraName,
		cfg.EyeDetectorName,
		cfg.EyeClassifierName,
		cfg.MotionDetectorName,
	}, nil, nil
}

type awakeClassifier struct {
	resource.AlwaysRebuild
	name    resource.Name
	logger  logging.Logger
	cfg     *Config
	workers *viamutils.StoppableWorkers

	camName          string
	cam              camera.Camera
	eyeDetector      vision.Service
	eyeClassifier    vision.Service
	motionDetector   vision.Service
	minEyeConfidence float64
	results          *Results

	properties vision.Properties
}

func NewAwakeClassifier(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (vision.Service, error) {
	name := rawConf.ResourceName()
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	s := &awakeClassifier{
		name:   name,
		logger: logger,
		cfg:    conf,
		properties: vision.Properties{
			ClassificationSupported: true,
			DetectionSupported:      false,
			ObjectPCDsSupported:     false,
		},
		minEyeConfidence: defaultMinEyeConfidence,
	}

	if conf.MinEyeConfidence != 0 {
		s.minEyeConfidence = conf.MinEyeConfidence
	}

	s.cam, err = camera.FromProvider(deps, conf.CameraName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get camera %v from dependencies", conf.CameraName)
	}
	s.camName = conf.CameraName

	s.eyeDetector, err = vision.FromProvider(deps, conf.EyeDetectorName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get eye detector %v from dependencies", conf.EyeDetectorName)
	}

	s.eyeClassifier, err = vision.FromProvider(deps, conf.EyeClassifierName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get eye classifier %v from dependencies", conf.EyeClassifierName)
	}

	s.motionDetector, err = vision.FromProvider(deps, conf.MotionDetectorName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get motion detector %v from dependencies", conf.MotionDetectorName)
	}

	s.results = NewResults()

	s.workers = viamutils.NewStoppableWorkers(context.Background())
	s.workers.Add(func(cancelCtx context.Context) {
		for {
			if err := s.runLoop(cancelCtx); err != nil {
				if strings.Contains(err.Error(), "context canceled") {
					return
				}
				s.logger.Errorw("background loop error", "error", err)
				continue
			}
			return
		}
	})

	return s, nil
}

// runLoop continuously processes frames until the context is cancelled.
// Camera errors are returned (triggering a retry in the worker); pipeline errors
// are non-fatal and only logged so transient ML failures don't crash the loop.
func (s *awakeClassifier) runLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := s.processFrame(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *awakeClassifier) processFrame(ctx context.Context) error {
	img, err := camera.DecodeImageFromCamera(ctx, s.cam, nil, nil)
	if err != nil {
		return errors.Wrapf(err, "camera %v error in background thread", s.camName)
	}

	cls, pipelineErr := runPipeline(ctx, img, s.camName, s.eyeDetector, s.eyeClassifier, s.motionDetector, s.minEyeConfidence, s.logger)
	if pipelineErr != nil {
		s.logger.Warnw("pipeline error", "error", pipelineErr)
		s.results.Store(img, nil)
		return nil
	}

	s.results.Store(img, cls)
	return nil
}

func (s *awakeClassifier) Name() resource.Name {
	return s.name
}

// ClassificationsFromCamera returns the latest cached awake classification.
// Confidence encodes the probability of being awake (0=asleep, 1=awake).
func (s *awakeClassifier) ClassificationsFromCamera(ctx context.Context, cameraName string, n int, extra map[string]interface{}) (classification.Classifications, error) {
	_, cls := s.results.Load()
	return cls, nil
}

func (s *awakeClassifier) Classifications(ctx context.Context, img image.Image, n int, extra map[string]interface{}) (classification.Classifications, error) {
	_, cls := s.results.Load()
	return cls, nil
}

func (s *awakeClassifier) DetectionsFromCamera(ctx context.Context, cameraName string, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, errUnimplemented
}

func (s *awakeClassifier) Detections(ctx context.Context, img image.Image, extra map[string]interface{}) ([]objdet.Detection, error) {
	return nil, errUnimplemented
}

func (s *awakeClassifier) GetObjectPointClouds(ctx context.Context, cameraName string, extra map[string]interface{}) ([]*vis.Object, error) {
	return nil, errUnimplemented
}

func (s *awakeClassifier) GetProperties(ctx context.Context, extra map[string]interface{}) (*vision.Properties, error) {
	return &s.properties, nil
}

func (s *awakeClassifier) CaptureAllFromCamera(ctx context.Context, cameraName string, opt viscapture.CaptureOptions, extra map[string]interface{}) (viscapture.VisCapture, error) {
	result := viscapture.VisCapture{}
	resImg, cls := s.results.Load()
	if opt.ReturnImage {
		result.Image = resImg
	}
	if opt.ReturnClassifications {
		result.Classifications = cls
	}
	return result, nil
}

func (s *awakeClassifier) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	command, ok := cmd["command"].(string)
	if !ok {
		return nil, errors.New("command must be a string")
	}

	switch command {
	case "get_last_result":
		_, cls := s.results.Load()
		if len(cls) == 0 {
			return map[string]interface{}{
				"class_name": "awake",
				"score":      float64(0),
			}, nil
		}
		return map[string]interface{}{
			"class_name": cls[0].Label(),
			"score":      float64(cls[0].Score()),
		}, nil
	default:
		return nil, errors.Errorf("unknown command: %s", command)
	}
}

func (s *awakeClassifier) Close(context.Context) error {
	s.workers.Stop()
	return nil
}

package babymonitor

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	viamutils "go.viam.com/utils"
)

var (
	AwakeClassifierModel = resource.NewModel("simone-kalmakis", "baby-monitor", "awake-classifier")
	errUnimplemented     = errors.New("unimplemented")
)

const defaultMinEyeConfidence = 0.5

func init() {
	resource.RegisterComponent(sensor.API, AwakeClassifierModel,
		resource.Registration[sensor.Sensor, *Config]{
			Constructor: NewAwakeClassifier,
		},
	)
}

// Config holds the configuration for the awake-classifier sensor.
type Config struct {
	CameraName         string  `json:"camera_name"`
	EyeDetectorName    string  `json:"eye_detector_name"`
	MotionDetectorName string  `json:"motion_detector_name"`
	AudioSensorName    string  `json:"audio_sensor_name"`
	MinEyeConfidence   float64 `json:"min_eye_confidence"`
}

// Validate returns all dependencies so viam-server starts them before this module.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.CameraName == "" {
		return nil, nil, errors.New("camera_name is required")
	}
	if cfg.EyeDetectorName == "" {
		return nil, nil, errors.New("eye_detector_name is required")
	}
	if cfg.MotionDetectorName == "" {
		return nil, nil, errors.New("motion_detector_name is required")
	}
	if cfg.AudioSensorName == "" {
		return nil, nil, errors.New("audio_sensor_name is required")
	}
	return []string{
		cfg.CameraName,
		cfg.EyeDetectorName,
		cfg.MotionDetectorName,
		cfg.AudioSensorName,
	}, nil, nil
}

type awakeClassifier struct {
	resource.AlwaysRebuild
	name    resource.Name
	logger  logging.Logger
	cfg     *Config
	workers *viamutils.StoppableWorkers

	camName          string
	eyeDetector      vision.Service
	motionDetector   vision.Service
	audioSensor      sensor.Sensor
	minEyeConfidence float64
	results          *Results
}

func NewAwakeClassifier(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (sensor.Sensor, error) {
	name := rawConf.ResourceName()
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	s := &awakeClassifier{
		name:             name,
		logger:           logger,
		cfg:              conf,
		camName:          conf.CameraName,
		minEyeConfidence: defaultMinEyeConfidence,
	}

	if conf.MinEyeConfidence != 0 {
		s.minEyeConfidence = conf.MinEyeConfidence
	}

	s.eyeDetector, err = vision.FromProvider(deps, conf.EyeDetectorName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get eye detector %v from dependencies", conf.EyeDetectorName)
	}

	s.motionDetector, err = vision.FromProvider(deps, conf.MotionDetectorName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get motion detector %v from dependencies", conf.MotionDetectorName)
	}

	s.audioSensor, err = sensor.FromProvider(deps, conf.AudioSensorName)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get audio sensor %v from dependencies", conf.AudioSensorName)
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

// runLoop continuously runs the fusion pipeline until the context is cancelled.
func (s *awakeClassifier) runLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := s.runOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *awakeClassifier) runOnce(ctx context.Context) error {
	isAwake, err := runPipeline(ctx, s.camName, s.eyeDetector, s.motionDetector, s.audioSensor, s.minEyeConfidence, s.logger)
	if err != nil {
		s.logger.Warnw("pipeline error", "error", err)
		return nil
	}
	s.results.Store(isAwake)
	return nil
}

func (s *awakeClassifier) Name() resource.Name {
	return s.name
}

// Readings returns the latest fused awake/asleep determination.
// Output keys: "is_awake" (bool).
func (s *awakeClassifier) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{
		"is_awake": s.results.Load(),
	}, nil
}

func (s *awakeClassifier) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	command, ok := cmd["command"].(string)
	if !ok {
		return nil, errors.New("command must be a string")
	}
	switch command {
	case "get_last_result":
		return map[string]interface{}{
			"is_awake": s.results.Load(),
		}, nil
	default:
		return nil, errors.Errorf("unknown command: %s", command)
	}
}

func (s *awakeClassifier) Close(context.Context) error {
	s.workers.Stop()
	return nil
}

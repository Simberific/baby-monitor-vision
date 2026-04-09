package babymonitor

import (
	"context"
	"image"
	"math/rand"

	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/services/vision"
	vis "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/classification"
	objdet "go.viam.com/rdk/vision/objectdetection"
	"go.viam.com/rdk/vision/viscapture"
)

var MockEyeClassifierModel = resource.NewModel("simone-kalmakis", "baby-monitor", "mock-eye-classifier")

func init() {
	resource.RegisterService(vision.API, MockEyeClassifierModel,
		resource.Registration[vision.Service, *MockEyeClassifierConfig]{
			Constructor: NewMockEyeClassifier,
		},
	)
}

// MockEyeClassifierConfig has no required fields — this model needs no dependencies.
type MockEyeClassifierConfig struct{}

func (c *MockEyeClassifierConfig) Validate(path string) ([]string, []string, error) {
	return nil, nil, nil
}

type mockEyeClassifier struct {
	resource.AlwaysRebuild
	name resource.Name
}

func NewMockEyeClassifier(_ context.Context, _ resource.Dependencies, rawConf resource.Config, _ logging.Logger) (vision.Service, error) {
	return &mockEyeClassifier{name: rawConf.ResourceName()}, nil
}

func (m *mockEyeClassifier) Name() resource.Name {
	return m.name
}

// Classifications returns a random open/closed label with a random confidence in [0.5, 1.0].
func (m *mockEyeClassifier) Classifications(_ context.Context, _ image.Image, _ int, _ map[string]interface{}) (classification.Classifications, error) {
	label := "closed"
	if rand.Float64() > 0.5 {
		label = "open"
	}
	return classification.Classifications{
		classification.NewClassification(0.5+rand.Float64()*0.5, label),
	}, nil
}

func (m *mockEyeClassifier) ClassificationsFromCamera(ctx context.Context, _ string, n int, extra map[string]interface{}) (classification.Classifications, error) {
	return m.Classifications(ctx, nil, n, extra)
}

func (m *mockEyeClassifier) DetectionsFromCamera(_ context.Context, _ string, _ map[string]interface{}) ([]objdet.Detection, error) {
	return nil, errUnimplemented
}

func (m *mockEyeClassifier) Detections(_ context.Context, _ image.Image, _ map[string]interface{}) ([]objdet.Detection, error) {
	return nil, errUnimplemented
}

func (m *mockEyeClassifier) GetObjectPointClouds(_ context.Context, _ string, _ map[string]interface{}) ([]*vis.Object, error) {
	return nil, errUnimplemented
}

func (m *mockEyeClassifier) GetProperties(_ context.Context, _ map[string]interface{}) (*vision.Properties, error) {
	return &vision.Properties{
		ClassificationSupported: true,
		DetectionSupported:      false,
		ObjectPCDsSupported:     false,
	}, nil
}

func (m *mockEyeClassifier) CaptureAllFromCamera(_ context.Context, _ string, _ viscapture.CaptureOptions, _ map[string]interface{}) (viscapture.VisCapture, error) {
	return viscapture.VisCapture{}, errUnimplemented
}

func (m *mockEyeClassifier) DoCommand(_ context.Context, _ map[string]interface{}) (map[string]interface{}, error) {
	return nil, errUnimplemented
}

func (m *mockEyeClassifier) Close(context.Context) error {
	return nil
}

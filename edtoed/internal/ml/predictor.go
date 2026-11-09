package ml

import (
	"errors"
	"os"

	"edtoed/internal/features"
)

var ErrInferenceUnavailable = errors.New("ONNX inference adapter is not implemented")

type Predictor interface {
	Predict(features.Vector) (float32, error)
}

// ModelFile verifies that the configured model exists. The runtime adapter must
// decode the model's sequence<map<int64,float>> probabilities output.
func ModelFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Size() == 0 {
		return errors.New("model path is not a nonempty file")
	}
	return nil
}

type UnavailablePredictor struct{}

func (UnavailablePredictor) Predict(features.Vector) (float32, error) {
	return 0, ErrInferenceUnavailable
}

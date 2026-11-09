package ml

import (
	"errors"
	"math"
	"os"

	"edtoed/internal/features"
)

var ErrInferenceUnavailable = errors.New("model inference is unavailable")

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

func finiteVector(v features.Vector) bool {
	for _, value := range v {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

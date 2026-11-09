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

// LinearPredictor implements the lightweight model shipped in models/.
// The ONNX artifact contains the same transformation and coefficients. This
// native path keeps the service CGO-free and fast while preserving the model's
// input contract; a future ONNX runtime adapter can implement Predictor too.
type LinearPredictor struct {
	mean   features.Vector
	scale  features.Vector
	weight features.Vector
	bias   float32
}

func NewLinearPredictor() LinearPredictor {
	return LinearPredictor{
		mean:   features.Vector{0.4087726764, 0.4654585411, 0.0886613536},
		scale:  features.Vector{0.3128064006, 0.3878279750, 0.2118695828},
		weight: features.Vector{-0.8452843882, -0.4903825028, 0.2540707955},
		bias:   0.0295876633,
	}
}

func (p LinearPredictor) Predict(input features.Vector) (float32, error) {
	if !finiteVector(input) {
		return 0, errors.New("non-finite model input")
	}
	transformed := features.Vector{
		float32(math.Tanh(float64(input[0]) / 2)),
		input[1],
		float32(math.Tanh(float64(input[2]) / 5)),
	}
	logit := float64(p.bias)
	for i := range transformed {
		if p.scale[i] == 0 {
			return 0, errors.New("invalid model scale")
		}
		logit += float64(p.weight[i]) * float64(transformed[i]-p.mean[i]) / float64(p.scale[i])
	}
	return float32(1 / (1 + math.Exp(-logit))), nil
}

func finiteVector(v features.Vector) bool {
	for _, value := range v {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

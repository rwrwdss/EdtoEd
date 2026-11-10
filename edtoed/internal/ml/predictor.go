package ml

import (
	"context"
	"errors"
	"math"
	"os"

	"edtoed/internal/features"
)

var ErrInferenceUnavailable = errors.New("model inference is unavailable")

type Predictor interface {
	Predict(context.Context, features.Vector) (float32, error)
}

func checkModelFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Size() == 0 {
		return errors.New("model path is not a nonempty file")
	}
	return nil
}

func finiteVector(v features.Vector) bool {
	for _, value := range v {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

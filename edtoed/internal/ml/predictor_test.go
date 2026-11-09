package ml

import (
	"testing"

	"edtoed/internal/features"
)

func TestLinearPredictorReturnsBoundedRisk(t *testing.T) {
	p := NewLinearPredictor()
	for _, input := range []features.Vector{{0, 0, 0}, {1, 0.8, 0}, {10, 0.2, 5}} {
		risk, err := p.Predict(input)
		if err != nil || risk < 0 || risk > 1 {
			t.Fatalf("input %v: risk=%v err=%v", input, risk, err)
		}
	}
}

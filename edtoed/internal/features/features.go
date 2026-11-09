package features

import "math"

type Vector [3]float32

// Build keeps the order expected by the ONNX model: relative clicks, score, delay.
func Build(clicks int32, medianClicks float64, score, delay float32) Vector {
	if medianClicks <= 0 || math.IsNaN(medianClicks) || math.IsInf(medianClicks, 0) {
		medianClicks = 1
	}
	return Vector{float32(float64(clicks) / medianClicks), score, delay}
}

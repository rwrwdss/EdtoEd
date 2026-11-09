package ml

import (
	"edtoed/internal/features"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeIntegration(t *testing.T) {
	if os.Getenv("TEST_ONNX") != "1" {
		t.Skip("set TEST_ONNX=1 with Python ONNX Runtime installed")
	}
	p, err := NewRuntime("../../models/edtoed_v1.onnx")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, tc := range []struct {
		x    features.Vector
		want float64
	}{
		{features.Vector{1, .8, 0}, .4263506185},
		{features.Vector{0, 0, 5}, .8588041263},
		{features.Vector{0, .5, -2}, .6083977804},
		{features.Vector{2, 1, 0}, .2703960569},
	} {
		got, err := p.Predict(tc.x)
		if err != nil || math.Abs(float64(got)-tc.want) > 1e-6 {
			t.Fatalf("got %v, %v; want %v", got, err, tc.want)
		}
	}
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Predict(features.Vector{1, .5, 0}); err == nil {
		t.Fatal("expected failure for interrupted worker")
	}
	if !p.Ready() {
		t.Fatal("worker did not recover")
	}
	path := filepath.Join(t.TempDir(), "broken.onnx")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if bad, err := NewRuntime(path); err == nil {
		bad.Close()
		t.Fatal("accepted invalid model")
	}
	p.Close()
	if _, err := p.Predict(features.Vector{}); err == nil {
		t.Fatal("accepted request after shutdown")
	}
}

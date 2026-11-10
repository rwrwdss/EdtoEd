//go:build unix

package ml

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"edtoed/internal/features"
)

func TestRuntimeReplacesStalledWorker(t *testing.T) {
	if os.Getenv("TEST_ONNX") != "1" {
		t.Skip("set TEST_ONNX=1 with Python ONNX Runtime installed")
	}
	p, err := NewRuntime(t.Context(), "../../models/edtoed_v1.onnx")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := p.Predict(ctx, features.Vector{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled inference: %v", err)
	}
	if !p.Ready(t.Context()) {
		t.Fatal("worker did not recover after timeout")
	}
}

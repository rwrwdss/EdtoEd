package ml

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"edtoed/internal/features"
)

//go:embed worker.py
var worker string

// Runtime owns a persistent ONNX worker. The gate keeps each pipe exchange
// exclusive and lets callers cancel while waiting for the worker.
type Runtime struct {
	gate   chan struct{}
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Reader
	closed bool
	path   string
	python string
}

func NewRuntime(ctx context.Context, path string) (*Runtime, error) {
	if err := checkModelFile(path); err != nil {
		return nil, err
	}
	python := os.Getenv("PYTHON_BIN")
	if python == "" {
		python = "python3"
	}
	p := &Runtime{path: path, python: python, gate: make(chan struct{}, 1)}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := p.start(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Runtime) start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.cmd = exec.Command(p.python, "-u", "-c", worker, p.path)
	p.cmd.Stderr = os.Stderr
	var err error
	p.input, err = p.cmd.StdinPipe()
	if err != nil {
		p.cmd = nil
		return err
	}
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		p.input.Close()
		p.cmd = nil
		return err
	}
	p.output = bufio.NewReader(out)
	if err = p.cmd.Start(); err != nil {
		p.input.Close()
		out.Close()
		p.cmd = nil
		return err
	}
	finish := interruptOnCancel(ctx, p.cmd.Process)
	line, err := p.output.ReadString('\n')
	finish()
	if ctx.Err() != nil || err != nil || line != "ready\n" {
		p.stop()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return fmt.Errorf("read worker handshake: %w", err)
		}
		return fmt.Errorf("unexpected worker handshake")
	}
	return nil
}

func (p *Runtime) Predict(ctx context.Context, v features.Vector) (float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case p.gate <- struct{}{}:
		defer func() { <-p.gate }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if p.closed || !finiteVector(v) {
		return 0, ErrInferenceUnavailable
	}
	if p.cmd == nil {
		if err := p.start(ctx); err != nil {
			return 0, fmt.Errorf("%w: start worker: %w", ErrInferenceUnavailable, err)
		}
	}
	finish := interruptOnCancel(ctx, p.cmd.Process)
	risk, err := p.predict(v)
	finish()
	if ctx.Err() != nil || err != nil {
		// An interrupted exchange may leave an unread reply in the pipe.
		// Start a clean worker on the next request.
		p.stop()
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, fmt.Errorf("%w: exchange with worker: %w", ErrInferenceUnavailable, err)
	}
	return risk, nil
}

func (p *Runtime) predict(v features.Vector) (float32, error) {
	if err := json.NewEncoder(p.input).Encode(v); err != nil {
		return 0, err
	}
	line, err := p.output.ReadBytes('\n')
	if err != nil {
		return 0, err
	}
	var risk *float32
	if err := json.Unmarshal(line, &risk); err != nil || risk == nil || *risk < 0 || *risk > 1 {
		return 0, fmt.Errorf("invalid worker response")
	}
	return *risk, nil
}

func (p *Runtime) Ready(ctx context.Context) bool {
	_, err := p.Predict(ctx, features.Vector{1, .5, 0})
	return err == nil
}

func (p *Runtime) Close() {
	p.gate <- struct{}{}
	defer func() { <-p.gate }()
	p.closed = true
	p.stop()
}

func (p *Runtime) stop() {
	if p.cmd == nil {
		return
	}
	_ = p.input.Close()
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
	p.cmd = nil
}

func interruptOnCancel(ctx context.Context, process *os.Process) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = process.Kill()
		close(done)
	})
	return func() {
		if !stop() {
			<-done
		}
	}
}

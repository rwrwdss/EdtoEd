package ml

import (
	"bufio"
	"edtoed/internal/features"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

//go:embed worker.py
var worker string

// Runtime owns a persistent CPU ONNX Runtime process. The model is loaded once.
type Runtime struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Reader
	closed bool
}

func NewRuntime(path string) (*Runtime, error) {
	if err := ModelFile(path); err != nil {
		return nil, err
	}
	python := os.Getenv("PYTHON_BIN")
	if python == "" {
		python = "python3"
	}
	p := &Runtime{cmd: exec.Command(python, "-u", "-c", worker, path)}
	p.cmd.Stderr = os.Stderr
	var err error
	p.input, err = p.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		p.input.Close()
		return nil, err
	}
	p.output = bufio.NewReader(out)
	if err = p.cmd.Start(); err != nil {
		p.input.Close()
		out.Close()
		return nil, err
	}
	timer := time.AfterFunc(30*time.Second, func() { _ = p.cmd.Process.Kill() })
	line, err := p.output.ReadString('\n')
	timer.Stop()
	if err != nil || line != "ready\n" {
		p.Close()
		return nil, fmt.Errorf("ONNX startup failed: %v", err)
	}
	return p, nil
}

func (p *Runtime) Predict(v features.Vector) (float32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !finiteVector(v) {
		return 0, ErrInferenceUnavailable
	}
	timer := time.AfterFunc(5*time.Second, func() { _ = p.cmd.Process.Kill() })
	defer timer.Stop()
	if err := json.NewEncoder(p.input).Encode(v); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInferenceUnavailable, err)
	}
	line, err := p.output.ReadBytes('\n')
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInferenceUnavailable, err)
	}
	var risk float32
	if err := json.Unmarshal(line, &risk); err != nil || risk < 0 || risk > 1 {
		return 0, ErrInferenceUnavailable
	}
	return risk, nil
}

func (p *Runtime) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	_ = p.input.Close()
	_ = p.cmd.Process.Kill()
	return p.cmd.Wait()
}

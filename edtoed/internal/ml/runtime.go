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
	path   string
	python string
}

func NewRuntime(path string) (*Runtime, error) {
	if err := ModelFile(path); err != nil {
		return nil, err
	}
	python := os.Getenv("PYTHON_BIN")
	if python == "" {
		python = "python3"
	}
	p := &Runtime{path: path, python: python}
	if err := p.start(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Runtime) start() error {
	p.cmd = exec.Command(p.python, "-u", "-c", worker, p.path)
	p.cmd.Stderr = os.Stderr
	var err error
	p.input, err = p.cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		p.input.Close()
		return err
	}
	p.output = bufio.NewReader(out)
	if err = p.cmd.Start(); err != nil {
		p.input.Close()
		out.Close()
		return err
	}
	process := p.cmd.Process
	timer := time.AfterFunc(30*time.Second, func() { _ = process.Kill() })
	line, err := p.output.ReadString('\n')
	timer.Stop()
	if err != nil || line != "ready\n" {
		_ = process.Kill()
		_ = p.cmd.Wait()
		_ = p.input.Close()
		return fmt.Errorf("ONNX startup failed: %v", err)
	}
	return nil
}

func (p *Runtime) Predict(v features.Vector) (float32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !finiteVector(v) {
		return 0, ErrInferenceUnavailable
	}
	process := p.cmd.Process
	timer := time.AfterFunc(5*time.Second, func() { _ = process.Kill() })
	risk, err := p.predict(v)
	timer.Stop()
	if err != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		_ = p.input.Close()
		if restartErr := p.start(); restartErr != nil {
			p.closed = true
			return 0, fmt.Errorf("%w: restart: %v", ErrInferenceUnavailable, restartErr)
		}
		return 0, fmt.Errorf("%w: worker restarted after failed request: %v", ErrInferenceUnavailable, err)
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
	var risk float32
	if err := json.Unmarshal(line, &risk); err != nil || risk < 0 || risk > 1 {
		return 0, fmt.Errorf("invalid worker response: %s", line)
	}
	return risk, nil
}

func (p *Runtime) Ready() bool {
	_, err := p.Predict(features.Vector{1, .5, 0})
	return err == nil
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

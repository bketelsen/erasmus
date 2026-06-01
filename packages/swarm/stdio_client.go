package swarm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StdioProcess is a long-lived subprocess controlled by newline-delimited JSON.
type StdioProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
}

// StdioProcessConfig configures a long-lived swarm child process.
type StdioProcessConfig struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
	Stderr     io.Writer
}

// StdioRequest is a JSON-line request sent to a swarm child process.
type StdioRequest struct {
	ID     string `json:"id,omitempty"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// StdioResponse is a JSON-line response returned by a swarm child process.
type StdioResponse struct {
	ID     string          `json:"id,omitempty"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// StartStdioProcess starts a long-lived swarm child subprocess.
func StartStdioProcess(ctx context.Context, cfg StdioProcessConfig) (*StdioProcess, error) {
	if cfg.Executable == "" {
		return nil, fmt.Errorf("swarm subprocess executable is required")
	}
	cmd := exec.CommandContext(ctx, cfg.Executable, cfg.Args...)
	cmd.Env = cfg.Env
	cmd.Dir = cfg.Dir
	cmd.Stderr = cfg.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	return &StdioProcess{cmd: cmd, stdin: stdin, stdout: bufio.NewScanner(stdout)}, nil
}

// Request sends one request and waits for the next response.
func (p *StdioProcess) Request(req StdioRequest) (StdioResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := json.Marshal(req)
	if err != nil {
		return StdioResponse{}, err
	}
	if _, err := p.stdin.Write(append(data, '\n')); err != nil {
		return StdioResponse{}, err
	}
	if !p.stdout.Scan() {
		if err := p.stdout.Err(); err != nil {
			return StdioResponse{}, err
		}
		return StdioResponse{}, io.EOF
	}
	var resp StdioResponse
	if err := json.Unmarshal(p.stdout.Bytes(), &resp); err != nil {
		return StdioResponse{}, err
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// Close closes stdin and waits for process exit.
func (p *StdioProcess) Close() error {
	_ = p.stdin.Close()
	return p.cmd.Wait()
}

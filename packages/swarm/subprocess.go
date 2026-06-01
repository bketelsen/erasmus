package swarm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// SubprocessRun describes a one-shot swarm child process invocation.
type SubprocessRun struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
	Stdout     io.Writer
	Stderr     io.Writer
}

// RunSubprocess starts a swarm child process, waits for it to exit, and forwards output.
func RunSubprocess(ctx context.Context, run SubprocessRun) error {
	if run.Executable == "" {
		return fmt.Errorf("swarm subprocess executable is required")
	}
	cmd := exec.CommandContext(ctx, run.Executable, run.Args...)
	cmd.Env = run.Env
	cmd.Dir = run.Dir
	var stderr bytes.Buffer
	if run.Stdout != nil {
		cmd.Stdout = run.Stdout
	}
	if run.Stderr != nil {
		cmd.Stderr = io.MultiWriter(run.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("swarm subprocess failed: %w: %s", err, stderr.String())
		}
		return fmt.Errorf("swarm subprocess failed: %w", err)
	}
	return nil
}

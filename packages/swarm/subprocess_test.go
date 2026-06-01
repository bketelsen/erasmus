package swarm

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestRunSubprocessForwardsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	var out bytes.Buffer
	err := RunSubprocess(context.Background(), SubprocessRun{
		Executable: "/bin/sh",
		Args:       []string{"-c", "printf hello"},
		Stdout:     &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunSubprocessIncludesStderrOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	err := RunSubprocess(context.Background(), SubprocessRun{
		Executable: "/bin/sh",
		Args:       []string{"-c", "printf boom >&2; exit 7"},
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v", err)
	}
}

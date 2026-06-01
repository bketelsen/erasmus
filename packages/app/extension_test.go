package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"erasmus/packages/app"
)

func TestExtensionListProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}
	path := filepath.Join(t.TempDir(), "ext.sh")
	script := `#!/usr/bin/env bash
printf '%s\n' '{"type":"hello","data":{"name":"test","version":"1"}}'
printf '%s\n' '{"type":"register_tool","data":{"name":"echo_ext","description":"echo extension"}}'
printf '%s\n' '{"type":"register_command","data":{"name":"hello","description":"hello command"}}'
while IFS= read -r line; do :; done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := app.ExtensionListProcess(context.Background(), &out, path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tool\techo_ext\techo extension") || !strings.Contains(out.String(), "command\thello\thello command") {
		t.Fatalf("output:\n%s", out.String())
	}
}

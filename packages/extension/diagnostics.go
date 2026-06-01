package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ringLog struct {
	mu     sync.Mutex
	lines  []string
	limit  int
	path   string
	file   *os.File
	closed bool
}

func newRingLog(limit int) *ringLog {
	if limit <= 0 {
		limit = 1
	}
	return &ringLog{limit: limit}
}

func newPersistentRingLog(limit int, path string) (*ringLog, error) {
	log := newRingLog(limit)
	if strings.TrimSpace(path) == "" {
		return log, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	log.path = path
	log.file = file
	return log, nil
}

func (r *ringLog) Add(line string) {
	r.AddSource("", line)
}

func (r *ringLog) AddSource(source, line string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) >= r.limit {
		copy(r.lines, r.lines[1:])
		r.lines[len(r.lines)-1] = line
	} else {
		r.lines = append(r.lines, line)
	}
	if r.file != nil && !r.closed {
		_ = json.NewEncoder(r.file).Encode(diagnosticEntry{Time: time.Now().UTC().Format(time.RFC3339Nano), Source: source, Message: line})
	}
}

func (r *ringLog) Lines() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func (r *ringLog) Path() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path
}

func (r *ringLog) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil || r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

type diagnosticEntry struct {
	Time    string `json:"time"`
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

func formatDiagnostics(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return "\nrecent extension diagnostics:\n  " + strings.Join(lines, "\n  ")
}

func formatDiagnosticsPath(path string) string {
	if path == "" {
		return ""
	}
	return "\nextension log: " + path
}

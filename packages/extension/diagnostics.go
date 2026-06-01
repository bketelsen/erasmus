package extension

import (
	"strings"
	"sync"
)

type ringLog struct {
	mu    sync.Mutex
	lines []string
	limit int
}

func newRingLog(limit int) *ringLog {
	if limit <= 0 {
		limit = 1
	}
	return &ringLog{limit: limit}
}

func (r *ringLog) Add(line string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) >= r.limit {
		copy(r.lines, r.lines[1:])
		r.lines[len(r.lines)-1] = line
		return
	}
	r.lines = append(r.lines, line)
}

func (r *ringLog) Lines() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func formatDiagnostics(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return "\nrecent extension diagnostics:\n  " + strings.Join(lines, "\n  ")
}

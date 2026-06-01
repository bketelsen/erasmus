package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bketelsen/erasmus/packages/config"
	"github.com/bketelsen/erasmus/packages/swarm"
)

// SwarmServerRecord is persisted metadata for a swarm socket server.
type SwarmServerRecord struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	Socket    string    `json:"socket"`
	CWD       string    `json:"cwd,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Model     string    `json:"model,omitempty"`
	Started   time.Time `json:"started"`
	LastSeen  time.Time `json:"last_seen"`
	Status    string    `json:"status"`
	Reachable bool      `json:"reachable,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// RegisterSwarmServer writes running swarm server metadata to the local registry.
func RegisterSwarmServer(ctx context.Context, name, socket string, cfg config.Config) (SwarmServerRecord, error) {
	if err := ctx.Err(); err != nil {
		return SwarmServerRecord{}, err
	}
	if name == "" {
		name = "default"
	}
	cwd := cfg.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return SwarmServerRecord{}, err
		}
	}
	now := time.Now()
	rec := SwarmServerRecord{Name: name, PID: os.Getpid(), Socket: socket, CWD: cwd, Provider: cfg.Provider, Model: cfg.Model, Started: now, LastSeen: now, Status: "running"}
	return rec, writeSwarmServerRecord(rec)
}

// MarkSwarmServerStopped marks a registry entry as stopped if it exists.
func MarkSwarmServerStopped(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rec, err := ReadSwarmServer(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	rec.LastSeen = time.Now()
	rec.Status = "stopped"
	return writeSwarmServerRecord(rec)
}

// ReadSwarmServer reads one server registry entry by name.
func ReadSwarmServer(name string) (SwarmServerRecord, error) {
	if name == "" {
		name = "default"
	}
	data, err := os.ReadFile(swarmServerRecordPath(name))
	if err != nil {
		return SwarmServerRecord{}, err
	}
	var rec SwarmServerRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return SwarmServerRecord{}, err
	}
	return rec, nil
}

// ListSwarmServers lists local swarm server registry entries.
func ListSwarmServers(ctx context.Context) ([]SwarmServerRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := swarmServerRegistryDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]SwarmServerRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var rec SwarmServerRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// CheckSwarmServers lists local swarm server registry entries and checks socket reachability.
func CheckSwarmServers(ctx context.Context) ([]SwarmServerRecord, error) {
	records, err := ListSwarmServers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		checkCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		_, err := swarm.SocketRequest(checkCtx, records[i].Socket, swarm.StdioRequest{Method: "status"})
		cancel()
		if err != nil {
			records[i].Reachable = false
			records[i].Error = err.Error()
			if records[i].Status == "running" {
				records[i].Status = "stale"
				_ = writeSwarmServerRecord(records[i])
			}
			continue
		}
		records[i].Reachable = true
		records[i].Error = ""
		records[i].Status = "running"
		records[i].LastSeen = time.Now()
		_ = writeSwarmServerRecord(records[i])
	}
	return records, nil
}

// PruneSwarmServers removes stopped/stale unreachable server registry entries.
func PruneSwarmServers(ctx context.Context) ([]SwarmServerRecord, error) {
	records, err := CheckSwarmServers(ctx)
	if err != nil {
		return nil, err
	}
	removed := []SwarmServerRecord{}
	for _, rec := range records {
		if rec.Reachable || (rec.Status != "stale" && rec.Status != "stopped") {
			continue
		}
		if err := os.Remove(swarmServerRecordPath(rec.Name)); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if strings.Contains(rec.Socket, string(os.PathSeparator)) {
			_ = os.Remove(rec.Socket)
		}
		removed = append(removed, rec)
	}
	return removed, nil
}

// ResolveSwarmSocket resolves either a direct socket address or a registry name.
func ResolveSwarmSocket(socket, name string) (string, error) {
	if socket != "" {
		return socket, nil
	}
	if name == "" {
		name = "default"
	}
	rec, err := ReadSwarmServer(name)
	if err != nil {
		return "", fmt.Errorf("swarm server %q not found: %w", name, err)
	}
	if rec.Socket == "" {
		return "", fmt.Errorf("swarm server %q has no socket", name)
	}
	return rec.Socket, nil
}

func writeSwarmServerRecord(rec SwarmServerRecord) error {
	if err := os.MkdirAll(swarmServerRegistryDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(swarmServerRecordPath(rec.Name), append(data, '\n'), 0o600)
}

func swarmServerRegistryDir() string {
	if dir := os.Getenv("ERASMUS_SWARM_REGISTRY_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(xdgStateHome(), "erasmus", "swarm", "servers")
}

func swarmServerRecordPath(name string) string {
	return filepath.Join(swarmServerRegistryDir(), safeSwarmServerName(name)+".json")
}

func safeSwarmServerName(name string) string {
	if name == "" {
		return "default"
	}
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "default"
	}
	return name
}

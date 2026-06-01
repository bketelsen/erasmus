package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erasmus/packages/config"
)

func TestSwarmServerRegistry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	old, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	rec, err := RegisterSwarmServer(context.Background(), "smoke", "/tmp/erasmus.sock", config.Config{Provider: "fake", Model: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "smoke" || rec.Socket == "" || rec.Status != "running" {
		t.Fatalf("record = %+v", rec)
	}
	addr, err := ResolveSwarmSocket("", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if addr != "/tmp/erasmus.sock" {
		t.Fatalf("addr = %q", addr)
	}
	list, err := ListSwarmServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "smoke" {
		t.Fatalf("list = %+v", list)
	}
	if err := MarkSwarmServerStopped(context.Background(), "smoke"); err != nil {
		t.Fatal(err)
	}
	stopped, err := ReadSwarmServer("smoke")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "stopped" {
		t.Fatalf("status = %q", stopped.Status)
	}
}

func TestSwarmServerRegistryUsesXDGState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	old, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	got := swarmServerRegistryDir()
	wantPrefix := filepath.Join(tmp, "state", "erasmus", "swarm", "servers")
	if got != wantPrefix {
		t.Fatalf("swarmServerRegistryDir() = %q, want %q", got, wantPrefix)
	}
	if strings.Contains(got, ".erasmus") {
		t.Fatalf("path %q still uses project-local .erasmus storage", got)
	}
}

func TestCheckSwarmServersMarksRunningRecordStale(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	old, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if _, err := RegisterSwarmServer(context.Background(), "dead", "/tmp/erasmus-definitely-dead.sock", config.Config{Provider: "fake", Model: "echo"}); err != nil {
		t.Fatal(err)
	}
	records, err := CheckSwarmServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	if records[0].Reachable || records[0].Status != "stale" || records[0].Error == "" {
		t.Fatalf("record = %+v", records[0])
	}
	persisted, err := ReadSwarmServer("dead")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != "stale" {
		t.Fatalf("persisted = %+v", persisted)
	}
	removed, err := PruneSwarmServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].Name != "dead" {
		t.Fatalf("removed = %+v", removed)
	}
	if _, err := ReadSwarmServer("dead"); !os.IsNotExist(err) {
		t.Fatalf("expected removed record, err=%v", err)
	}
}

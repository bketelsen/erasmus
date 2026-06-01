package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"erasmus/packages/auth"
	"erasmus/packages/config"
)

func TestServeSwarmListenerAcrossConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() {
		errc <- ServeSwarmListener(ctx, ln, config.Config{Provider: "fake", Model: "echo"}, auth.NewMemoryStore())
	}()
	addr := ln.Addr().String()
	if err := socketRequest(addr, `{"id":"1","method":"spawn","params":{"id":"main","task":"socket first","memory":true}}`); err != nil {
		t.Fatal(err)
	}
	if err := socketRequest(addr, `{"id":"2","method":"wait","params":{"id":"main"}}`); err != nil {
		t.Fatal(err)
	}
	if err := socketRequest(addr, `{"id":"3","method":"list"}`); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-errc:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func socketRequest(addr, line string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := fmt.Fprintln(conn, line); err != nil {
		return err
	}
	got, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(got, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

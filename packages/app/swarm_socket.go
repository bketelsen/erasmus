package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"erasmus/packages/auth"
	"erasmus/packages/config"
)

// ServeSwarmSocket serves a long-lived swarm controller over a local socket.
func ServeSwarmSocket(ctx context.Context, addr string, cfg config.Config, store auth.Store) error {
	if addr == "" {
		return fmt.Errorf("socket address is required")
	}
	network := "tcp"
	if strings.Contains(addr, string(os.PathSeparator)) {
		network = "unix"
		_ = os.Remove(addr)
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	if network == "unix" {
		defer os.Remove(addr)
	}
	return ServeSwarmListener(ctx, ln, cfg, store)
}

// ServeSwarmListener serves a swarm controller on an existing listener.
func ServeSwarmListener(ctx context.Context, ln net.Listener, cfg config.Config, store auth.Store) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	controller, err := newSwarmController(ctx, cfg, store)
	if err != nil {
		return err
	}
	controller.shutdown = cancel
	controller.socket = ln.Addr().String()
	defer controller.close(ctx)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
		case <-done:
		}
	}()
	defer close(done)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func() {
			defer conn.Close()
			_ = controller.serveConn(ctx, conn, conn)
		}()
	}
}

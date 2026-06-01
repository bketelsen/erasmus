package swarm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

// SocketRequest sends one JSON-line swarm request to a socket server.
func SocketRequest(ctx context.Context, addr string, req StdioRequest) (StdioResponse, error) {
	if addr == "" {
		return StdioResponse{}, fmt.Errorf("socket address is required")
	}
	network := "tcp"
	if strings.Contains(addr, string(os.PathSeparator)) {
		network = "unix"
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return StdioResponse{}, err
	}
	defer conn.Close()
	data, err := json.Marshal(req)
	if err != nil {
		return StdioResponse{}, err
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return StdioResponse{}, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return StdioResponse{}, err
	}
	var resp StdioResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return StdioResponse{}, err
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

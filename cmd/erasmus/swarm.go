package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bketelsen/erasmus/packages/app"
	"github.com/bketelsen/erasmus/packages/swarm"
)

func newSwarmCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "swarm",
		Short: "Run and control background agents",
	}
	cmd.AddCommand(
		newSwarmRunCommand(),
		newSwarmChildCommand(),
		newSwarmServeCommand(),
		newSwarmPSCommand(),
		newSwarmPruneCommand(),
		newSwarmDashboardCommand("dashboard"),
		newSwarmDashboardCommand("watch"),
		newSwarmLogsCommand(),
		newSwarmAttachCommand(),
		newSwarmSpawnCommand(),
		newSwarmSendCommand(),
		newSwarmAgentCommand("wait"),
		newSwarmAgentCommand("stop"),
		newSwarmServerCommand("list"),
		newSwarmServerCommand("status"),
		newSwarmServerCommand("close"),
	)
	return cmd
}

type swarmTargetOptions struct {
	socket string
	name   string
}

func addSwarmTargetFlags(cmd *cobra.Command, target *swarmTargetOptions) {
	cmd.Flags().StringVar(&target.socket, "socket", "", "swarm socket address")
	cmd.Flags().StringVar(&target.name, "name", "", "registered swarm server name")
}

func resolveSwarmTarget(target swarmTargetOptions) (string, error) {
	return app.ResolveSwarmSocket(target.socket, target.name)
}

func newSwarmRunCommand() *cobra.Command {
	var opts app.SwarmRunOptions
	cmd := &cobra.Command{
		Use:   "run [flags] <task>",
		Short: "Run a swarm task",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.SessionPath != "" && opts.SessionDir != "" {
				return fmt.Errorf("use only one of --session or --session-dir")
			}
			opts.Task = strings.Join(args, " ")
			opts.Out = cmd.OutOrStdout()
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			return app.RunSwarmConfigured(context.Background(), opts, cfg, authStore())
		},
	}
	cmd.Flags().BoolVar(&opts.MemorySession, "memory", false, "use an in-memory session")
	cmd.Flags().BoolVar(&opts.Subprocess, "subprocess", false, "run through a subprocess")
	cmd.Flags().StringVar(&opts.SessionPath, "session", "", "durable JSONL session path")
	cmd.Flags().StringVar(&opts.SessionDir, "session-dir", "", "durable JSONL session directory")
	return cmd
}

func newSwarmChildCommand() *cobra.Command {
	var stdio bool
	var socket string
	var opts app.SwarmRunOptions
	cmd := &cobra.Command{
		Use:    "child",
		Short:  "Run a swarm child process",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			switch {
			case stdio:
				return app.ServeSwarmStdio(context.Background(), os.Stdin, cmd.OutOrStdout(), cfg, authStore())
			case socket != "":
				return app.ServeSwarmSocket(context.Background(), socket, cfg, authStore())
			default:
				if opts.SessionPath != "" && opts.SessionDir != "" {
					return fmt.Errorf("use only one of --session or --session-dir")
				}
				opts.Task = strings.Join(args, " ")
				opts.Out = cmd.OutOrStdout()
				opts.Subprocess = false
				return app.RunSwarmConfigured(context.Background(), opts, cfg, authStore())
			}
		},
	}
	cmd.Flags().BoolVar(&stdio, "stdio", false, "serve swarm child over stdio")
	cmd.Flags().StringVar(&socket, "socket", "", "serve swarm child over socket")
	cmd.Flags().BoolVar(&opts.MemorySession, "memory", false, "use an in-memory session")
	cmd.Flags().StringVar(&opts.SessionPath, "session", "", "durable JSONL session path")
	cmd.Flags().StringVar(&opts.SessionDir, "session-dir", "", "durable JSONL session directory")
	return cmd
}

func newSwarmServeCommand() *cobra.Command {
	var target swarmTargetOptions
	cmd := &cobra.Command{
		Use:   "serve --socket <addr> [--name <name>]",
		Short: "Serve a swarm socket",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if target.socket == "" {
				return fmt.Errorf("usage: erasmus swarm serve --socket <addr> [--name <name>]")
			}
			cfg, err := loadConfig(context.Background())
			if err != nil {
				return err
			}
			if _, err := app.RegisterSwarmServer(context.Background(), target.name, target.socket, cfg); err != nil {
				return err
			}
			defer app.MarkSwarmServerStopped(context.Background(), target.name)
			return app.ServeSwarmSocket(context.Background(), target.socket, cfg, authStore())
		},
	}
	addSwarmTargetFlags(cmd, &target)
	return cmd
}

func newSwarmPSCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List registered swarm servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.CheckSwarmServers(context.Background())
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
}

func newSwarmPruneCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Prune stale swarm servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.PruneSwarmServers(context.Background())
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
}

func newSwarmDashboardCommand(name string) *cobra.Command {
	var target swarmTargetOptions
	once := false
	interval := time.Second
	cmd := &cobra.Command{
		Use:   name + " [name]",
		Short: "Render swarm dashboard",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if target.socket == "" && target.name == "" && len(args) == 1 {
				target.name = args[0]
			}
			addr, err := resolveSwarmTarget(target)
			if err != nil {
				return err
			}
			for {
				resp, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: "status"})
				if err != nil {
					return err
				}
				if err := printSwarmDashboard(cmd.OutOrStdout(), resp.Result); err != nil {
					return err
				}
				if once {
					return nil
				}
				time.Sleep(interval)
			}
		},
	}
	addSwarmTargetFlags(cmd, &target)
	cmd.Flags().BoolVar(&once, "once", false, "render once and exit")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "refresh interval")
	return cmd
}

func newSwarmLogsCommand() *cobra.Command {
	var target swarmTargetOptions
	cmd := &cobra.Command{
		Use:   "logs [name] [agent]",
		Short: "Print swarm agent event logs",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := ""
			if target.socket == "" && target.name == "" && len(args) > 0 {
				target.name = args[0]
				args = args[1:]
			}
			if len(args) > 0 {
				agentID = args[0]
			}
			addr, err := resolveSwarmTarget(target)
			if err != nil {
				return err
			}
			status, err := swarmStatus(addr)
			if err != nil {
				return err
			}
			for _, agent := range status.Agents {
				if agentID == "" || agent.ID == agentID {
					if agent.EventLog == "" {
						return fmt.Errorf("swarm agent %q has no event log", agent.ID)
					}
					data, err := os.ReadFile(agent.EventLog)
					if err != nil {
						return err
					}
					fmt.Fprint(cmd.OutOrStdout(), string(data))
					return nil
				}
			}
			return fmt.Errorf("swarm agent %q not found", agentID)
		},
	}
	addSwarmTargetFlags(cmd, &target)
	return cmd
}

func newSwarmAttachCommand() *cobra.Command {
	var target swarmTargetOptions
	cmd := &cobra.Command{
		Use:   "attach [name] [agent]",
		Short: "Attach to a swarm agent",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := ""
			if target.socket == "" && target.name == "" && len(args) > 0 {
				target.name = args[0]
				args = args[1:]
			}
			if len(args) > 0 {
				agentID = args[0]
			}
			return runSwarmAttach(cmd, target, agentID)
		},
	}
	addSwarmTargetFlags(cmd, &target)
	return cmd
}

func newSwarmSpawnCommand() *cobra.Command {
	var target swarmTargetOptions
	id := ""
	memory := false
	cmd := &cobra.Command{
		Use:   "spawn [flags] <task>",
		Short: "Spawn a swarm agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("usage: erasmus swarm spawn [--socket <addr>|--name <name>] [--id <id>] [--memory] <task>")
			}
			addr, err := resolveSwarmTarget(target)
			if err != nil {
				return err
			}
			resp, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: "spawn", Params: map[string]any{"id": id, "task": strings.Join(args, " "), "memory": memory}})
			if err != nil {
				return err
			}
			return printRawResult(cmd.OutOrStdout(), resp.Result)
		},
	}
	addSwarmTargetFlags(cmd, &target)
	cmd.Flags().StringVar(&id, "id", "", "agent id")
	cmd.Flags().BoolVar(&memory, "memory", false, "use an in-memory session")
	return cmd
}

func newSwarmSendCommand() *cobra.Command {
	var target swarmTargetOptions
	cmd := &cobra.Command{
		Use:   "send [flags] <id> <text>",
		Short: "Send text to a swarm agent",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := resolveSwarmTarget(target)
			if err != nil {
				return err
			}
			resp, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: "send", Params: map[string]any{"id": args[0], "text": strings.Join(args[1:], " ")}})
			if err != nil {
				return err
			}
			return printRawResult(cmd.OutOrStdout(), resp.Result)
		},
	}
	addSwarmTargetFlags(cmd, &target)
	return cmd
}

func newSwarmAgentCommand(method string) *cobra.Command {
	var target swarmTargetOptions
	cmd := &cobra.Command{
		Use:   method + " [flags] <id>",
		Short: method + " a swarm agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := resolveSwarmTarget(target)
			if err != nil {
				return err
			}
			resp, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: method, Params: map[string]any{"id": args[0]}})
			if err != nil {
				return err
			}
			return printRawResult(cmd.OutOrStdout(), resp.Result)
		},
	}
	addSwarmTargetFlags(cmd, &target)
	return cmd
}

func newSwarmServerCommand(method string) *cobra.Command {
	var target swarmTargetOptions
	cmd := &cobra.Command{
		Use:   method + " [flags] [name]",
		Short: method + " a swarm server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if target.socket == "" && target.name == "" && len(args) == 1 {
				target.name = args[0]
			}
			addr, err := resolveSwarmTarget(target)
			if err != nil {
				return err
			}
			resp, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: method})
			if err != nil {
				return err
			}
			return printRawResult(cmd.OutOrStdout(), resp.Result)
		},
	}
	addSwarmTargetFlags(cmd, &target)
	return cmd
}

func printJSON(out io.Writer, result any) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(data))
	return nil
}

func printRawResult(out io.Writer, result []byte) error {
	if len(result) > 0 {
		fmt.Fprintln(out, string(result))
	} else {
		fmt.Fprintln(out, "{}")
	}
	return nil
}

func printSwarmDashboard(out io.Writer, data []byte) error {
	var status struct {
		PID      int `json:"pid"`
		Socket   string
		CWD      string
		Provider string
		Model    string
		Uptime   string
		Agents   []struct {
			ID            string `json:"id"`
			Task          string `json:"task"`
			State         string `json:"state"`
			Running       bool   `json:"running"`
			Updated       string `json:"updated"`
			Messages      int    `json:"messages"`
			PendingTools  int    `json:"pending_tools"`
			Events        int    `json:"events"`
			LastEventType string `json:"last_event_type"`
			Provider      string `json:"provider"`
			Model         string `json:"model"`
			SessionID     string `json:"session_id"`
			Error         string `json:"error"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "server pid=%d socket=%s uptime=%s provider=%s model=%s\n", status.PID, status.Socket, status.Uptime, status.Provider, status.Model); err != nil {
		return err
	}
	if status.CWD != "" {
		if _, err := fmt.Fprintf(out, "cwd=%s\n", status.CWD); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "agents:"); err != nil {
		return err
	}
	if len(status.Agents) == 0 {
		_, err := fmt.Fprintln(out, "  none")
		return err
	}
	for _, agent := range status.Agents {
		state := agent.State
		if state == "" {
			state = "settled"
			if agent.Running {
				state = "running"
			}
		}
		if agent.Error != "" {
			state = "error: " + agent.Error
		}
		modelName := agent.Model
		if agent.Provider != "" && agent.Model != "" {
			modelName = agent.Provider + "/" + agent.Model
		}
		if _, err := fmt.Fprintf(out, "  %s\t%s\tmsgs=%d\ttools=%d\tevents=%d\tlast=%s\tsession=%s\tmodel=%s\tupdated=%s\ttask=%s\n", agent.ID, state, agent.Messages, agent.PendingTools, agent.Events, agent.LastEventType, agent.SessionID, modelName, agent.Updated, agent.Task); err != nil {
			return err
		}
	}
	return nil
}

func runSwarmAttach(cmd *cobra.Command, target swarmTargetOptions, agentID string) error {
	addr, err := resolveSwarmTarget(target)
	if err != nil {
		return err
	}
	status, err := swarmStatus(addr)
	if err != nil {
		return err
	}
	if agentID == "" && len(status.Agents) > 0 {
		agentID = status.Agents[0].ID
	}
	if agentID == "" {
		agentID = "main"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "attached to %s. type /quit to exit.\n", agentID)
	_ = printSwarmDashboard(cmd.OutOrStdout(), status.Raw)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/quit" || line == "/exit" {
			return nil
		}
		if _, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: "send", Params: map[string]any{"id": agentID, "text": line}}); err != nil {
			return err
		}
		if _, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: "wait", Params: map[string]any{"id": agentID}}); err != nil {
			return err
		}
		status, err := swarmStatus(addr)
		if err != nil {
			return err
		}
		if err := printSwarmDashboard(cmd.OutOrStdout(), status.Raw); err != nil {
			return err
		}
	}
	return scanner.Err()
}

type swarmStatusSnapshot struct {
	Raw    []byte
	Agents []struct {
		ID       string `json:"id"`
		EventLog string `json:"event_log"`
	} `json:"agents"`
}

func swarmStatus(addr string) (swarmStatusSnapshot, error) {
	resp, err := swarm.SocketRequest(context.Background(), addr, swarm.StdioRequest{Method: "status"})
	if err != nil {
		return swarmStatusSnapshot{}, err
	}
	var status swarmStatusSnapshot
	status.Raw = resp.Result
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		return swarmStatusSnapshot{}, err
	}
	return status, nil
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bketelsen/erasmus/packages/event"
	"github.com/bketelsen/erasmus/packages/harness"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/session/memory"
	"github.com/bketelsen/erasmus/packages/swarm"
)

type options struct {
	Task     string
	StateDir string
}

type workflowResult struct {
	Agents  map[string]swarm.Snapshot
	Summary string
}

func main() {
	opts := parseFlags(os.Args[1:])
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result, err := runWorkflow(ctx, opts)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
	fmt.Print(result.Summary)
	if opts.StateDir != "" {
		fmt.Fprintf(os.Stdout, "\nevent logs: %s\n", filepath.Join(opts.StateDir, "events"))
	}
}

func parseFlags(args []string) options {
	fs := flag.NewFlagSet("swarm-workflow", flag.ExitOnError)
	opts := options{Task: "Design a small transcript export feature", StateDir: filepath.Join(".erasmus", "swarm-workflow")}
	fs.StringVar(&opts.Task, "task", opts.Task, "workflow task to plan, review, and execute")
	fs.StringVar(&opts.StateDir, "state", opts.StateDir, "directory for swarm event logs")
	_ = fs.Parse(args)
	if rest := fs.Args(); len(rest) > 0 {
		opts.Task = strings.Join(rest, " ")
	}
	return opts
}

func runWorkflow(ctx context.Context, opts options) (workflowResult, error) {
	if strings.TrimSpace(opts.Task) == "" {
		return workflowResult{}, fmt.Errorf("task is required")
	}
	eventLogDir := ""
	if opts.StateDir != "" {
		eventLogDir = filepath.Join(opts.StateDir, "events")
	}
	server, err := swarm.New(swarm.Config{
		EventLogDir: eventLogDir,
		Factory: func(ctx context.Context, req swarm.SpawnRequest) (*harness.Harness, error) {
			return harness.New(ctx, harness.Config{
				Session: memory.New(req.ID),
				Model: model.Model{
					Provider:      "fake",
					ID:            req.ID,
					DisplayName:   req.ID + " workflow agent",
					ContextWindow: 32000,
					MaxOutput:     2048,
				},
				Stream:   roleStream(req.ID),
				MaxSteps: 4,
			})
		},
	})
	if err != nil {
		return workflowResult{}, err
	}

	plan, err := runNamedAgent(ctx, server, "planner", "Plan this task:\n\n"+opts.Task)
	if err != nil {
		return workflowResult{}, err
	}
	review, err := runNamedAgent(ctx, server, "reviewer", "Review this plan for gaps and risks:\n\n"+plan)
	if err != nil {
		return workflowResult{}, err
	}
	execution, err := runNamedAgent(ctx, server, "executor", "Execute the approved plan.\n\nTask:\n"+opts.Task+"\n\nPlan:\n"+plan+"\n\nReview:\n"+review)
	if err != nil {
		return workflowResult{}, err
	}

	list, err := server.List(ctx)
	if err != nil {
		return workflowResult{}, err
	}
	agents := make(map[string]swarm.Snapshot, len(list))
	for _, snap := range list {
		agents[snap.ID] = snap
	}
	return workflowResult{
		Agents: agents,
		Summary: strings.Join([]string{
			"## Plan",
			plan,
			"## Review",
			review,
			"## Execution",
			execution,
			"",
		}, "\n\n"),
	}, nil
}

func runNamedAgent(ctx context.Context, server *swarm.Swarm, name, prompt string) (string, error) {
	agent, err := server.Spawn(ctx, swarm.SpawnRequest{ID: name, Task: prompt})
	if err != nil {
		return "", err
	}
	if err := agent.Wait(ctx); err != nil {
		return "", err
	}
	return collectAssistantText(agent.Events()), nil
}

func collectAssistantText(events []event.Event) string {
	var b strings.Builder
	for _, ev := range events {
		if delta, ok := ev.(event.MessageDelta); ok {
			b.WriteString(delta.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func roleStream(role string) provider.StreamFunc {
	return func(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
		text := responseForRole(role, userText(req))
		return streamEvents(provider.MessageStart{MessageID: role + "-message"}, provider.TextDelta{Text: text}, provider.MessageEnd{StopReason: "end_turn"}), nil
	}
}

func responseForRole(role, input string) string {
	task := extractTask(input)
	switch role {
	case "planner":
		return "planner: Plan\n" +
			"1. Clarify the requested outcome.\n" +
			"2. Identify files and tests touched by the change.\n" +
			"3. Implement the smallest verifiable slice.\n" +
			"4. Run focused tests, then full CI.\n\n" +
			"Task: " + task
	case "reviewer":
		return "reviewer: Review\n" +
			"- The plan is appropriately incremental.\n" +
			"- Confirm test coverage before implementation.\n" +
			"- Keep the executor scoped to the requested workflow.\n\n" +
			"Reviewed task: " + task
	case "executor":
		return "executor: Execution\n" +
			"- Created an implementation checklist from the reviewed plan.\n" +
			"- Preserved reviewer constraints while sequencing the work.\n" +
			"- Ready for a real provider/tools-backed execution pass.\n\n" +
			"Executed task: " + task
	default:
		return role + ": " + task
	}
}

func extractTask(input string) string {
	lines := strings.Split(input, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasSuffix(line, ":") {
			return line
		}
	}
	return strings.TrimSpace(input)
}

func userText(req provider.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != message.RoleUser {
			continue
		}
		for _, c := range req.Messages[i].Content {
			if text, ok := c.(message.Text); ok {
				return text.Text
			}
		}
	}
	return ""
}

func streamEvents(events ...provider.Event) <-chan provider.Event {
	ch := make(chan provider.Event, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch
}

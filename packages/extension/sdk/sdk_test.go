package sdk_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"erasmus/packages/extension/proto"
	"erasmus/packages/extension/sdk"
	"erasmus/packages/skill"
)

func TestRunWithIORegistersAndDispatchesToolsAndCommands(t *testing.T) {
	toolCall, err := proto.EncodeFrame("tool_call", "tool-1", proto.ToolCall{ID: "tool-1", Name: "echo", Args: json.RawMessage(`{"text":"hello"}`)})
	if err != nil {
		t.Fatal(err)
	}
	commandCall, err := proto.EncodeFrame("command_call", "command-1", proto.CommandCall{ID: "command-1", Name: "hello", Input: json.RawMessage(`{"name":"Ada"}`)})
	if err != nil {
		t.Fatal(err)
	}
	input := encodeLines(t, toolCall, commandCall)
	var output bytes.Buffer

	err = sdk.RunWithIO(context.Background(), sdk.Extension{
		Name:    "test-extension",
		Version: "v1",
		Tools: []sdk.Tool{{
			Name:        "echo",
			Description: "echoes text",
			Schema:      json.RawMessage(`{"type":"object"}`),
			Handler: func(ctx context.Context, args json.RawMessage) (sdk.ToolResult, error) {
				var in struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return sdk.ErrorResult(err.Error()), nil
				}
				return sdk.TextResult("tool " + in.Text), nil
			},
		}},
		Commands: []sdk.Command{{
			Name:        "hello",
			Description: "prints a greeting",
			Handler: func(ctx context.Context, input json.RawMessage) ([]proto.HostAction, error) {
				var in struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(input, &in); err != nil {
					return nil, err
				}
				return []proto.HostAction{sdk.PrintAction("hello " + in.Name)}, nil
			},
		}},
	}, strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}

	frames := decodeLines(t, output.String())
	if got, want := frameTypes(frames), []string{"hello", "register_tool", "register_command", "tool_result", "command_result"}; !equalStrings(got, want) {
		t.Fatalf("frame types = %v, want %v", got, want)
	}
	toolResult := decodeToolResult(t, frames[3])
	if toolResult.ID != "tool-1" || toolResult.Error != "" || !strings.Contains(string(frames[3].Data), `"text":"tool hello"`) {
		t.Fatalf("tool result frame = %+v data=%s", toolResult, frames[3].Data)
	}
	var commandResult proto.CommandResult
	if err := proto.DecodeData(frames[4], &commandResult); err != nil {
		t.Fatal(err)
	}
	if commandResult.ID != "command-1" || commandResult.Error != "" || len(commandResult.Actions) != 1 || commandResult.Actions[0].Type != "print" || !strings.Contains(string(commandResult.Actions[0].Data), "hello Ada") {
		t.Fatalf("command result = %+v", commandResult)
	}
}

func TestRunWithIOSubscribesAndHandlesEvents(t *testing.T) {
	runtimeEvent, err := proto.EncodeFrame("event", "settled", proto.Event{Type: "settled", Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	err = sdk.RunWithIO(context.Background(), sdk.Extension{
		Name:   "test-extension",
		Events: []string{"settled"},
		OnEvent: func(ctx context.Context, ev proto.Event) ([]proto.HostAction, error) {
			if ev.Type != "settled" {
				t.Fatalf("event type = %q", ev.Type)
			}
			return []proto.HostAction{sdk.PrintAction("settled")}, nil
		},
	}, strings.NewReader(encodeLines(t, runtimeEvent)), &output)
	if err != nil {
		t.Fatal(err)
	}

	frames := decodeLines(t, output.String())
	if got, want := frameTypes(frames), []string{"hello", "subscribe", "host_action"}; !equalStrings(got, want) {
		t.Fatalf("frame types = %v, want %v", got, want)
	}
	var action proto.HostAction
	if err := proto.DecodeData(frames[2], &action); err != nil {
		t.Fatal(err)
	}
	if action.Type != "print" || !strings.Contains(string(action.Data), "settled") {
		t.Fatalf("action = %+v", action)
	}
}

func TestRunWithIOSubscribesAndHandlesHooks(t *testing.T) {
	hookCall, err := proto.EncodeFrame("hook_call", "hook-1", proto.HookCall{ID: "hook-1", Hook: "provider_request"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	err = sdk.RunWithIO(context.Background(), sdk.Extension{
		Name:  "test-extension",
		Hooks: []string{"provider_request"},
		OnHook: func(ctx context.Context, call proto.HookCall) (proto.HookResult, error) {
			if call.ID != "hook-1" || call.Hook != "provider_request" {
				t.Fatalf("call = %+v", call)
			}
			return sdk.DenyHookResult(call.ID, "blocked"), nil
		},
	}, strings.NewReader(encodeLines(t, hookCall)), &output)
	if err != nil {
		t.Fatal(err)
	}

	frames := decodeLines(t, output.String())
	if got, want := frameTypes(frames), []string{"hello", "subscribe_hooks", "hook_result"}; !equalStrings(got, want) {
		t.Fatalf("frame types = %v, want %v", got, want)
	}
	var result proto.HookResult
	if err := proto.DecodeData(frames[2], &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != "hook-1" || !result.Deny || result.Error != "blocked" {
		t.Fatalf("hook result = %+v", result)
	}
}

func TestSetActiveToolsAction(t *testing.T) {
	action := sdk.SetActiveToolsAction("read", "write")
	if action.Type != "set_active_tools" {
		t.Fatalf("type = %q", action.Type)
	}
	if !strings.Contains(string(action.Data), `"read"`) || !strings.Contains(string(action.Data), `"write"`) {
		t.Fatalf("data = %s", action.Data)
	}
}

func TestSetResourcesAction(t *testing.T) {
	action := sdk.SetResourcesAction([]string{"read"}, []skill.Skill{{Name: "review", Body: "Review carefully."}})
	if action.Type != "set_resources" {
		t.Fatalf("type = %q", action.Type)
	}
	if !strings.Contains(string(action.Data), `"active_tools":["read"]`) || !strings.Contains(string(action.Data), `"name":"review"`) {
		t.Fatalf("data = %s", action.Data)
	}
}

func TestSavePointAction(t *testing.T) {
	action := sdk.SavePointAction("before-change", map[string]string{"path": "main.go"})
	if action.Type != "save_point" {
		t.Fatalf("type = %q", action.Type)
	}
	if !strings.Contains(string(action.Data), `"label":"before-change"`) || !strings.Contains(string(action.Data), `"path":"main.go"`) {
		t.Fatalf("data = %s", action.Data)
	}
}

func TestRunWithIOReturnsProtocolErrorsAsFrames(t *testing.T) {
	toolCall, err := proto.EncodeFrame("tool_call", "tool-1", proto.ToolCall{ID: "tool-1", Name: "fail", Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	err = sdk.RunWithIO(context.Background(), sdk.Extension{
		Name: "test-extension",
		Tools: []sdk.Tool{{
			Name: "fail",
			Handler: func(ctx context.Context, args json.RawMessage) (sdk.ToolResult, error) {
				return sdk.ToolResult{}, errors.New("boom")
			},
		}},
	}, strings.NewReader(encodeLines(t, toolCall)), &output)
	if err != nil {
		t.Fatal(err)
	}

	frames := decodeLines(t, output.String())
	result := decodeToolResult(t, frames[len(frames)-1])
	if result.ID != "tool-1" || result.Error != "boom" || !result.Result.IsError {
		t.Fatalf("result = %+v", result)
	}
}

func decodeToolResult(t *testing.T, frame proto.Frame) struct {
	ID     string `json:"id"`
	Error  string `json:"error,omitempty"`
	Result struct {
		IsError bool `json:"is_error,omitempty"`
		Content []struct {
			Text string `json:"text,omitempty"`
		} `json:"content,omitempty"`
	} `json:"result"`
} {
	t.Helper()
	var result struct {
		ID     string `json:"id"`
		Error  string `json:"error,omitempty"`
		Result struct {
			IsError bool `json:"is_error,omitempty"`
			Content []struct {
				Text string `json:"text,omitempty"`
			} `json:"content,omitempty"`
		} `json:"result"`
	}
	if err := json.Unmarshal(frame.Data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func encodeLines(t *testing.T, frames ...proto.Frame) string {
	t.Helper()
	var b strings.Builder
	for _, frame := range frames {
		data, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func decodeLines(t *testing.T, data string) []proto.Frame {
	t.Helper()
	var frames []proto.Frame
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		var frame proto.Frame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return frames
}

func frameTypes(frames []proto.Frame) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		out = append(out, frame.Type)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

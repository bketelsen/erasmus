package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"erasmus/packages/extension/proto"
	"erasmus/packages/extension/sdk"
)

func TestExampleExtensionRunsToolAndCommand(t *testing.T) {
	toolCall, err := proto.EncodeFrame("tool_call", "tool-1", proto.ToolCall{ID: "tool-1", Name: "echo_go", Args: json.RawMessage(`{"text":"hi"}`)})
	if err != nil {
		t.Fatal(err)
	}
	commandCall, err := proto.EncodeFrame("command_call", "command-1", proto.CommandCall{ID: "command-1", Name: "hello_go", Input: json.RawMessage(`{"name":"Erasmus"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	if err := sdk.RunWithIO(context.Background(), newExtension(), strings.NewReader(encodeFrames(t, toolCall, commandCall)), &output); err != nil {
		t.Fatal(err)
	}

	frames := decodeFrames(t, output.String())
	if got, want := frameTypes(frames), []string{"hello", "register_tool", "register_command", "tool_result", "command_result"}; !equalStrings(got, want) {
		t.Fatalf("frame types = %v, want %v", got, want)
	}
	if !strings.Contains(string(frames[3].Data), `"text":"echo: hi"`) {
		t.Fatalf("tool result data = %s", frames[3].Data)
	}
	var commandResult proto.CommandResult
	if err := proto.DecodeData(frames[4], &commandResult); err != nil {
		t.Fatal(err)
	}
	if len(commandResult.Actions) != 1 || commandResult.Actions[0].Type != "print" || !strings.Contains(string(commandResult.Actions[0].Data), "hello Erasmus") {
		t.Fatalf("command result = %+v", commandResult)
	}
}

func encodeFrames(t *testing.T, frames ...proto.Frame) string {
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

func decodeFrames(t *testing.T, data string) []proto.Frame {
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

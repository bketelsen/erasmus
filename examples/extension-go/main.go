package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/bketelsen/erasmus/packages/extension/proto"
	"github.com/bketelsen/erasmus/packages/extension/sdk"
)

func main() {
	if err := sdk.Run(context.Background(), newExtension()); err != nil {
		log.Fatal(err)
	}
}

func newExtension() sdk.Extension {
	return sdk.Extension{
		Name:    "go-example",
		Version: "v0",
		Events:  []string{"settled"},
		OnEvent: func(ctx context.Context, ev proto.Event) ([]proto.HostAction, error) {
			if ev.Type == "settled" {
				return []proto.HostAction{sdk.PrintAction("go extension saw settled")}, nil
			}
			return nil, nil
		},
		Tools: []sdk.Tool{{
			Name:        "echo_go",
			Description: "Echo text from a Go SDK extension.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
			Handler: func(ctx context.Context, args json.RawMessage) (sdk.ToolResult, error) {
				var in struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return sdk.ErrorResult(err.Error()), nil
				}
				return sdk.TextResult("echo: " + strings.TrimSpace(in.Text)), nil
			},
		}},
		Commands: []sdk.Command{{
			Name:        "hello_go",
			Description: "Print a greeting from a Go SDK extension.",
			Handler: func(ctx context.Context, input json.RawMessage) ([]proto.HostAction, error) {
				var in struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(input, &in); err != nil {
					return nil, err
				}
				name := strings.TrimSpace(in.Name)
				if name == "" {
					name = "world"
				}
				return []proto.HostAction{sdk.PrintAction("hello " + name)}, nil
			},
		}},
	}
}

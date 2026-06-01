// Package openai adapts OpenAI-compatible chat completion streams to Erasmus provider events.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Config configures an OpenAI-compatible client.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client streams OpenAI-compatible chat completions.
type Client struct {
	apiKey string
	base   string
	http   *http.Client
}

// New creates an OpenAI-compatible provider client.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openai api key is required")
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{apiKey: cfg.APIKey, base: base, http: hc}, nil
}

// Name returns the provider name.
func (c *Client) Name() string { return "openai" }

// Stream sends a chat completion request and returns normalized provider events.
func (c *Client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	body, err := c.requestBody(req)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Authorization", "Bearer "+c.apiKey)
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	out := make(chan provider.Event, 16)
	go c.readStream(resp.Body, out)
	return out, nil
}

type chatRequest struct {
	Model         string        `json:"model"`
	Messages      []chatMessage `json:"messages"`
	Stream        bool          `json:"stream"`
	StreamOptions any           `json:"stream_options,omitempty"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	Tools         []chatTool    `json:"tools,omitempty"`
	ToolChoice    string        `json:"tool_choice,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (c *Client) requestBody(req provider.Request) ([]byte, error) {
	modelID := req.Model.ID
	if modelID == "" {
		return nil, fmt.Errorf("openai model id is required")
	}
	msgs := make([]chatMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.System) != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	for _, msg := range req.Messages {
		converted := convertMessage(msg)
		if converted == nil {
			continue
		}
		msgs = append(msgs, *converted)
	}
	body := chatRequest{Model: modelID, Messages: msgs, Stream: true, StreamOptions: map[string]bool{"include_usage": true}, MaxTokens: req.MaxTokens}
	if len(req.Tools) > 0 {
		body.Tools = make([]chatTool, 0, len(req.Tools))
		for _, spec := range req.Tools {
			body.Tools = append(body.Tools, chatTool{Type: "function", Function: chatFunction{Name: spec.Name, Description: spec.Description, Parameters: spec.Schema}})
		}
		body.ToolChoice = "auto"
	}
	return json.Marshal(body)
}

func convertMessage(msg message.Message) *chatMessage {
	content := textContent(msg.Content)
	switch msg.Role {
	case message.RoleUser, message.RoleSystem:
		if strings.TrimSpace(content) == "" {
			return nil
		}
		return &chatMessage{Role: string(msg.Role), Content: content}
	case message.RoleAssistant:
		out := chatMessage{Role: "assistant", Content: content}
		for _, part := range msg.Content {
			call, ok := part.(message.ToolCall)
			if !ok {
				continue
			}
			out.ToolCalls = append(out.ToolCalls, chatToolCall{ID: call.ID, Type: "function", Function: chatToolCallFunction{Name: call.Name, Arguments: call.Arguments}})
		}
		if strings.TrimSpace(out.Content) == "" && len(out.ToolCalls) == 0 {
			return nil
		}
		return &out
	case message.RoleTool:
		for _, part := range msg.Content {
			result, ok := part.(message.ToolResult)
			if !ok {
				continue
			}
			return &chatMessage{Role: "tool", ToolCallID: result.CallID, Content: textContent(result.Content)}
		}
		if strings.TrimSpace(content) == "" {
			return nil
		}
		return &chatMessage{Role: "user", Content: content}
	default:
		if strings.TrimSpace(content) == "" {
			return nil
		}
		return &chatMessage{Role: "user", Content: content}
	}
}

func textContent(parts []message.Content) string {
	var out []string
	for _, c := range parts {
		switch v := c.(type) {
		case message.Text:
			out = append(out, v.Text)
		case message.ToolResult:
			out = append(out, textContent(v.Content))
		}
	}
	return strings.Join(out, "\n")
}

type streamChunk struct {
	ID      string `json:"id,omitempty"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta,omitempty"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices,omitempty"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens,omitempty"`
		CompletionTokens int `json:"completion_tokens,omitempty"`
		TotalTokens      int `json:"total_tokens,omitempty"`
	} `json:"usage,omitempty"`
}

type pendingToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func pendingToolsInOrder(calls map[int]*pendingToolCall) []*pendingToolCall {
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]*pendingToolCall, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, calls[index])
	}
	return out
}

func (c *Client) readStream(body io.ReadCloser, out chan<- provider.Event) {
	defer close(out)
	defer body.Close()
	started := false
	pendingTools := map[int]*pendingToolCall{}
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			out <- provider.Error{Err: err.Error()}
			continue
		}
		if len(chunk.Choices) > 0 && !started {
			out <- provider.MessageStart{MessageID: chunk.ID}
			started = true
		}
		if chunk.Usage != nil {
			out <- provider.Usage{Usage: model.Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				out <- provider.TextDelta{Text: choice.Delta.Content}
			}
			for _, tc := range choice.Delta.ToolCalls {
				pending := pendingTools[tc.Index]
				if pending == nil {
					pending = &pendingToolCall{}
					pendingTools[tc.Index] = pending
				}
				if tc.ID != "" {
					pending.ID = tc.ID
				}
				if tc.Function.Name != "" {
					pending.Name += tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					pending.Arguments += tc.Function.Arguments
				}
			}
			if choice.FinishReason != "" {
				if len(pendingTools) > 0 {
					for _, pending := range pendingToolsInOrder(pendingTools) {
						if pending.ID == "" || pending.Name == "" {
							out <- provider.Error{Err: "openai stream ended with incomplete tool call"}
							continue
						}
						args := json.RawMessage(pending.Arguments)
						if len(args) == 0 {
							args = json.RawMessage(`{}`)
						}
						out <- provider.ToolCall{ID: pending.ID, Name: pending.Name, Arguments: args}
					}
					pendingTools = map[int]*pendingToolCall{}
				}
				out <- provider.MessageEnd{StopReason: choice.FinishReason}
				started = false
			}
		}
	}
	if err := scanner.Err(); err != nil {
		out <- provider.Error{Err: err.Error()}
		return
	}
	if started {
		out <- provider.MessageEnd{StopReason: "stop"}
	}
}

var _ provider.Client = (*Client)(nil)

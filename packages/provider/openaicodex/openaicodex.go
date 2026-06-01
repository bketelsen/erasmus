// Package openaicodex adapts ChatGPT Codex subscription streams to Erasmus provider events.
package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/provider"
)

const defaultBaseURL = "https://chatgpt.com/backend-api/codex/responses"

// Config configures the Codex subscription client.
type Config struct {
	AccessToken string
	AccountID   string
	BaseURL     string
	HTTPClient  *http.Client
	Originator  string
}

// Client streams ChatGPT Codex responses.
type Client struct {
	token      string
	accountID  string
	baseURL    string
	originator string
	http       *http.Client
}

// New creates a Codex subscription client.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("openai-codex access token is required")
	}
	if strings.TrimSpace(cfg.AccountID) == "" {
		return nil, fmt.Errorf("openai-codex account id is required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	originator := cfg.Originator
	if originator == "" {
		originator = "erasmus"
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{token: cfg.AccessToken, accountID: cfg.AccountID, baseURL: base, originator: originator, http: hc}, nil
}

// Name returns provider name.
func (c *Client) Name() string { return "openai-codex" }

// Stream sends a Responses-style request to ChatGPT Codex and normalizes SSE events.
func (c *Client) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	body, err := json.Marshal(buildRequest(req))
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("content-type", "application/json")
	hreq.Header.Set("accept", "text/event-stream")
	hreq.Header.Set("authorization", "Bearer "+c.token)
	hreq.Header.Set("chatgpt-account-id", c.accountID)
	hreq.Header.Set("openai-beta", "responses=experimental")
	hreq.Header.Set("originator", c.originator)
	hreq.Header.Set("user-agent", fmt.Sprintf("erasmus (%s %s)", runtime.GOOS, runtime.GOARCH))
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai-codex request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	out := make(chan provider.Event, 16)
	go readStream(resp.Body, out)
	return out, nil
}

type codexRequest struct {
	Model             string         `json:"model"`
	Store             bool           `json:"store"`
	Stream            bool           `json:"stream"`
	Instructions      string         `json:"instructions,omitempty"`
	Input             []any          `json:"input"`
	Tools             []responseTool `json:"tools,omitempty"`
	ParallelToolCalls bool           `json:"parallel_tool_calls"`
	Include           []string       `json:"include,omitempty"`
}

type responseTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type inputMessage struct {
	Role    string      `json:"role"`
	Content []inputText `json:"content"`
}

type inputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type functionCallInput struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type functionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

func buildRequest(req provider.Request) codexRequest {
	body := codexRequest{Model: req.Model.ID, Store: false, Stream: true, Instructions: req.System, ParallelToolCalls: true, Include: []string{"reasoning.encrypted_content"}}
	for _, spec := range req.Tools {
		body.Tools = append(body.Tools, responseTool{Type: "function", Name: spec.Name, Description: spec.Description, Parameters: spec.Schema})
	}
	for _, msg := range req.Messages {
		body.Input = append(body.Input, inputItems(msg)...)
	}
	return body
}

func inputItems(msg message.Message) []any {
	text := textContent(msg.Content)
	switch msg.Role {
	case message.RoleUser, message.RoleSystem:
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []any{inputMessage{Role: string(msg.Role), Content: []inputText{{Type: "input_text", Text: text}}}}
	case message.RoleAssistant:
		var out []any
		if strings.TrimSpace(text) != "" {
			out = append(out, inputMessage{Role: "assistant", Content: []inputText{{Type: "output_text", Text: text}}})
		}
		for _, part := range msg.Content {
			call, ok := part.(message.ToolCall)
			if !ok {
				continue
			}
			out = append(out, functionCallInput{Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: string(call.Arguments)})
		}
		return out
	case message.RoleTool:
		for _, part := range msg.Content {
			result, ok := part.(message.ToolResult)
			if !ok {
				continue
			}
			return []any{functionCallOutput{Type: "function_call_output", CallID: result.CallID, Output: textContent(result.Content)}}
		}
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []any{inputMessage{Role: "user", Content: []inputText{{Type: "input_text", Text: text}}}}
	default:
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []any{inputMessage{Role: "user", Content: []inputText{{Type: "input_text", Text: text}}}}
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

type ssePayload struct {
	Type        string `json:"type"`
	OutputIndex int    `json:"output_index,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	Delta       string `json:"delta,omitempty"`
	Arguments   string `json:"arguments,omitempty"`
	Item        struct {
		ID        string `json:"id,omitempty"`
		Type      string `json:"type,omitempty"`
		CallID    string `json:"call_id,omitempty"`
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"item,omitempty"`
	Response struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response,omitempty"`
}

type pendingResponseToolCall struct {
	ID        string
	CallID    string
	Name      string
	Arguments string
}

func responseToolKey(p ssePayload) string {
	if p.Item.ID != "" {
		return p.Item.ID
	}
	if p.ItemID != "" {
		return p.ItemID
	}
	return fmt.Sprintf("index:%d", p.OutputIndex)
}

func readStream(body io.ReadCloser, out chan<- provider.Event) {
	defer close(out)
	defer body.Close()
	started := false
	sawToolCall := false
	pendingTools := map[string]*pendingResponseToolCall{}
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var p ssePayload
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			out <- provider.Error{Err: err.Error()}
			continue
		}
		switch p.Type {
		case "response.output_item.added":
			if p.Item.Type == "function_call" {
				key := responseToolKey(p)
				pendingTools[key] = &pendingResponseToolCall{ID: p.Item.ID, CallID: p.Item.CallID, Name: p.Item.Name, Arguments: p.Item.Arguments}
				continue
			}
			if !started {
				out <- provider.MessageStart{}
				started = true
			}
		case "response.function_call_arguments.delta":
			key := responseToolKey(p)
			pending := pendingTools[key]
			if pending == nil {
				pending = &pendingResponseToolCall{ID: p.ItemID}
				pendingTools[key] = pending
			}
			pending.Arguments += p.Delta
		case "response.function_call_arguments.done":
			key := responseToolKey(p)
			pending := pendingTools[key]
			if pending == nil {
				pending = &pendingResponseToolCall{ID: p.ItemID}
				pendingTools[key] = pending
			}
			if p.Arguments != "" {
				pending.Arguments = p.Arguments
			}
		case "response.output_item.done":
			if p.Item.Type == "function_call" {
				key := responseToolKey(p)
				pending := pendingTools[key]
				if pending == nil {
					pending = &pendingResponseToolCall{}
				}
				if p.Item.ID != "" {
					pending.ID = p.Item.ID
				}
				if p.Item.CallID != "" {
					pending.CallID = p.Item.CallID
				}
				if p.Item.Name != "" {
					pending.Name = p.Item.Name
				}
				if p.Item.Arguments != "" {
					pending.Arguments = p.Item.Arguments
				}
				if pending.CallID == "" || pending.Name == "" {
					out <- provider.Error{Err: "openai-codex stream ended with incomplete tool call"}
					continue
				}
				args := json.RawMessage(pending.Arguments)
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				out <- provider.ToolCall{ID: pending.CallID, Name: pending.Name, Arguments: args}
				delete(pendingTools, key)
				sawToolCall = true
			}
		case "response.output_text.delta":
			if !started {
				out <- provider.MessageStart{}
				started = true
			}
			out <- provider.TextDelta{Text: p.Delta}
		case "response.completed", "response.done":
			out <- provider.Usage{Usage: model.Usage{InputTokens: p.Response.Usage.InputTokens, OutputTokens: p.Response.Usage.OutputTokens}}
			if len(pendingTools) > 0 {
				for key, pending := range pendingTools {
					if pending.CallID == "" || pending.Name == "" {
						out <- provider.Error{Err: "openai-codex stream completed with incomplete tool call"}
						continue
					}
					args := json.RawMessage(pending.Arguments)
					if len(args) == 0 {
						args = json.RawMessage(`{}`)
					}
					out <- provider.ToolCall{ID: pending.CallID, Name: pending.Name, Arguments: args}
					delete(pendingTools, key)
					sawToolCall = true
				}
			}
			if started || sawToolCall {
				stop := "stop"
				if sawToolCall {
					stop = "tool_calls"
				}
				out <- provider.MessageEnd{StopReason: stop}
				started = false
				sawToolCall = false
			}
		case "response.failed":
			out <- provider.Error{Err: p.Response.Error.Message}
		}
	}
	if err := scanner.Err(); err != nil {
		out <- provider.Error{Err: err.Error()}
	}
	if started {
		out <- provider.MessageEnd{StopReason: "stop"}
	}
}

var _ provider.Client = (*Client)(nil)

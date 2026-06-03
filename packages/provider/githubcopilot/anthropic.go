package githubcopilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bketelsen/erasmus/packages/auth"
	"github.com/bketelsen/erasmus/packages/message"
	"github.com/bketelsen/erasmus/packages/model"
	"github.com/bketelsen/erasmus/packages/provider"
	"github.com/bketelsen/erasmus/packages/tool"
)

// AnthropicMessagesClient streams Copilot Claude models through an Anthropic Messages-compatible API.
type AnthropicMessagesClient struct {
	token string
	base  string
	http  *http.Client
}

// NewAnthropicMessages creates a Copilot client for Anthropic Messages-compatible Claude models.
func NewAnthropicMessages(cfg Config) (*AnthropicMessagesClient, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("github-copilot access token is required")
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &AnthropicMessagesClient{token: cfg.AccessToken, base: baseURL, http: hc}, nil
}

// Name returns provider name.
func (c *AnthropicMessagesClient) Name() string { return "github-copilot" }

// Stream sends an Anthropic Messages request and returns normalized provider events.
func (c *AnthropicMessagesClient) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	body, err := json.Marshal(buildAnthropicRequest(req))
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Authorization", "Bearer "+c.token)
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "text/event-stream")
	hreq.Header.Set("Anthropic-Version", "2023-06-01")
	for k, v := range auth.GitHubCopilotStaticHeaders() {
		hreq.Header.Set(k, v)
	}
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github-copilot anthropic request failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	out := make(chan provider.Event, 16)
	go readAnthropicStream(resp.Body, out)
	return out, nil
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

func buildAnthropicRequest(req provider.Request) anthropicRequest {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = req.Model.MaxOutput
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}
	body := anthropicRequest{Model: req.Model.ID, System: req.System, MaxTokens: maxTokens, Stream: true}
	for _, msg := range req.Messages {
		converted := anthropicMessageFor(msg)
		if converted == nil {
			continue
		}
		body.Messages = append(body.Messages, *converted)
	}
	for _, spec := range req.Tools {
		body.Tools = append(body.Tools, anthropicToolFromSpec(spec))
	}
	return body
}

func anthropicMessageFor(msg message.Message) *anthropicMessage {
	role := string(msg.Role)
	if role != "assistant" {
		role = "user"
	}
	parts := anthropicContentFor(msg.Content)
	if len(parts) == 0 {
		return nil
	}
	return &anthropicMessage{Role: role, Content: parts}
}

func anthropicContentFor(parts []message.Content) []anthropicContent {
	out := make([]anthropicContent, 0, len(parts))
	for _, part := range parts {
		switch v := part.(type) {
		case message.Text:
			if strings.TrimSpace(v.Text) != "" {
				out = append(out, anthropicContent{Type: "text", Text: v.Text})
			}
		case message.ToolCall:
			input := v.Arguments
			if len(input) == 0 {
				input = json.RawMessage(`{}`)
			}
			out = append(out, anthropicContent{Type: "tool_use", ID: v.ID, Name: v.Name, Input: input})
		case message.ToolResult:
			out = append(out, anthropicContent{Type: "tool_result", ToolUseID: v.CallID, Content: anthropicTextContent(v.Content)})
		}
	}
	return out
}

func anthropicToolFromSpec(spec tool.Spec) anthropicTool {
	schema := spec.Schema
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object"}`)
	}
	return anthropicTool{Name: spec.Name, Description: spec.Description, InputSchema: schema}
}

func anthropicTextContent(parts []message.Content) string {
	var out []string
	for _, c := range parts {
		switch v := c.(type) {
		case message.Text:
			out = append(out, v.Text)
		case message.ToolResult:
			out = append(out, anthropicTextContent(v.Content))
		}
	}
	return strings.Join(out, "\n")
}

type anthropicStreamPayload struct {
	Type    string `json:"type"`
	Index   int    `json:"index,omitempty"`
	Message struct {
		ID string `json:"id,omitempty"`
	} `json:"message,omitempty"`
	ContentBlock struct {
		Type  string          `json:"type,omitempty"`
		ID    string          `json:"id,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content_block,omitempty"`
	Delta struct {
		Type        string `json:"type,omitempty"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
		StopReason  string `json:"stop_reason,omitempty"`
	} `json:"delta,omitempty"`
	Usage struct {
		InputTokens  int `json:"input_tokens,omitempty"`
		OutputTokens int `json:"output_tokens,omitempty"`
	} `json:"usage,omitempty"`
}

type pendingAnthropicTool struct {
	ID        string
	Name      string
	Arguments string
}

func readAnthropicStream(body io.ReadCloser, out chan<- provider.Event) {
	defer close(out)
	defer body.Close()
	pendingTools := map[int]*pendingAnthropicTool{}
	stopReason := "stop"
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var p anthropicStreamPayload
		if err := json.Unmarshal([]byte(data), &p); err != nil {
			out <- provider.Error{Err: err.Error()}
			continue
		}
		switch p.Type {
		case "message_start":
			out <- provider.MessageStart{MessageID: p.Message.ID}
		case "content_block_start":
			if p.ContentBlock.Type == "tool_use" {
				pendingTools[p.Index] = &pendingAnthropicTool{ID: p.ContentBlock.ID, Name: p.ContentBlock.Name, Arguments: initialAnthropicToolInput(p.ContentBlock.Input)}
			}
		case "content_block_delta":
			switch p.Delta.Type {
			case "text_delta":
				out <- provider.TextDelta{Text: p.Delta.Text}
			case "input_json_delta":
				pending := pendingTools[p.Index]
				if pending == nil {
					pending = &pendingAnthropicTool{}
					pendingTools[p.Index] = pending
				}
				pending.Arguments += p.Delta.PartialJSON
			}
		case "content_block_stop":
			pending := pendingTools[p.Index]
			if pending != nil {
				if pending.ID == "" || pending.Name == "" {
					out <- provider.Error{Err: "github-copilot anthropic stream ended with incomplete tool call"}
					delete(pendingTools, p.Index)
					continue
				}
				args := json.RawMessage(pending.Arguments)
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				out <- provider.ToolCall{ID: pending.ID, Name: pending.Name, Arguments: args}
				delete(pendingTools, p.Index)
			}
		case "message_delta":
			if p.Delta.StopReason != "" {
				stopReason = p.Delta.StopReason
			}
			if p.Usage.InputTokens != 0 || p.Usage.OutputTokens != 0 {
				out <- provider.Usage{Usage: model.Usage{InputTokens: p.Usage.InputTokens, OutputTokens: p.Usage.OutputTokens}}
			}
		case "message_stop":
			out <- provider.MessageEnd{StopReason: stopReason}
		case "error":
			out <- provider.Error{Err: strings.TrimSpace(data)}
		}
	}
	if err := scanner.Err(); err != nil {
		out <- provider.Error{Err: err.Error()}
	}
}

func initialAnthropicToolInput(input json.RawMessage) string {
	trimmed := strings.TrimSpace(string(input))
	switch trimmed {
	case "", "null", "{}":
		return ""
	default:
		return trimmed
	}
}

var _ provider.Client = (*AnthropicMessagesClient)(nil)

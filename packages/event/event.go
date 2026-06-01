// Package event defines canonical runtime events emitted by the loop, agent, and harness.
package event

import (
	"encoding/json"

	"erasmus/packages/message"
	"erasmus/packages/model"
	"erasmus/packages/skill"
	"erasmus/packages/tool"
)

// Event is implemented by every Erasmus runtime event.
type Event interface {
	Type() string
}

// AgentStart marks the beginning of a loop run.
type AgentStart struct{}

func (AgentStart) Type() string { return "agent_start" }

// AgentEnd marks the end of a loop run.
type AgentEnd struct {
	Messages []message.Message `json:"messages,omitempty"`
}

func (AgentEnd) Type() string { return "agent_end" }

// TurnStart marks the beginning of a provider/tool turn.
type TurnStart struct {
	Step int `json:"step"`
}

func (TurnStart) Type() string { return "turn_start" }

// TurnEnd marks the end of a provider/tool turn.
type TurnEnd struct {
	Step int    `json:"step"`
	Stop string `json:"stop,omitempty"`
	Err  string `json:"err,omitempty"`
}

func (TurnEnd) Type() string { return "turn_end" }

// MessageStart marks the beginning of an assistant message.
type MessageStart struct {
	Message message.Message `json:"message"`
}

func (MessageStart) Type() string { return "message_start" }

// MessageDelta carries an incremental text delta for a message.
type MessageDelta struct {
	MessageID string `json:"message_id,omitempty"`
	Text      string `json:"text"`
}

func (MessageDelta) Type() string { return "message_delta" }

// MessageEnd marks a committed message.
type MessageEnd struct {
	Message message.Message `json:"message"`
}

func (MessageEnd) Type() string { return "message_end" }

// ToolExecutionStart marks the beginning of tool execution.
type ToolExecutionStart struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

func (ToolExecutionStart) Type() string { return "tool_execution_start" }

// ToolExecutionProgress carries tool progress text.
type ToolExecutionProgress struct {
	ID   string `json:"id"`
	Text string `json:"text,omitempty"`
}

func (ToolExecutionProgress) Type() string { return "tool_execution_progress" }

// ToolExecutionEnd marks the end of tool execution.
type ToolExecutionEnd struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Result  tool.Result `json:"result"`
	IsError bool        `json:"is_error,omitempty"`
}

func (ToolExecutionEnd) Type() string { return "tool_execution_end" }

// Usage reports provider usage.
type Usage struct {
	Usage      model.Usage `json:"usage"`
	Cumulative model.Usage `json:"cumulative"`
}

func (Usage) Type() string { return "usage" }

// ResourcesUpdate reports prompt, skill, and tool resource changes.
type ResourcesUpdate struct {
	Skills      []skill.Skill `json:"skills,omitempty"`
	Tools       []tool.Spec   `json:"tools,omitempty"`
	ActiveTools []string      `json:"active_tools,omitempty"`
}

func (ResourcesUpdate) Type() string { return "resources_update" }

// ModelUpdate reports a runtime model change.
type ModelUpdate struct {
	Model model.Model `json:"model"`
}

func (ModelUpdate) Type() string { return "model_update" }

// ReasoningUpdate reports a runtime reasoning level change.
type ReasoningUpdate struct {
	Reasoning string `json:"reasoning,omitempty"`
}

func (ReasoningUpdate) Type() string { return "reasoning_update" }

// SessionTree reports a session tree navigation/update.
type SessionTree struct {
	LeafID string `json:"leaf_id,omitempty"`
	Action string `json:"action,omitempty"`
}

func (SessionTree) Type() string { return "session_tree" }

// SessionCompact reports a session compaction.
type SessionCompact struct {
	Summary      string `json:"summary"`
	TokensBefore int    `json:"tokens_before,omitempty"`
}

func (SessionCompact) Type() string { return "session_compact" }

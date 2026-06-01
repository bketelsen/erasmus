package tools

import (
	"erasmus/packages/sandbox"
	"erasmus/packages/tool"
)

// DefaultRegistry returns the built-in coding tools for a sandbox policy.
func DefaultRegistry(policy sandbox.Policy) tool.Registry {
	return tool.NewRegistry(
		NewReadTool(policy),
		NewWriteTool(policy),
		NewEditTool(policy),
		NewBashTool(policy),
	)
}

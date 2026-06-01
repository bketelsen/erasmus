package tools

import (
	"github.com/bketelsen/erasmus/packages/sandbox"
	"github.com/bketelsen/erasmus/packages/tool"
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

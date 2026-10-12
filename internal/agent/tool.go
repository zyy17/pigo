package agent

import (
	"context"

	"github.com/zyy17/pigo/internal/ai"
)

// Tool is a capability the model may call.
// Execute does the work. The schema sent to the model is Schema, which
// has no function pointer.
type Tool struct {
	Name        string
	Label       string
	Description string
	Parameters  ai.Schema
	Execute     func(ctx context.Context, args []byte) (string, error)
}

// Schema is the model-facing description of the tool.
func (t Tool) Schema() ai.Tool {
	return ai.Tool{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  t.Parameters,
	}
}

package coding

import (
	"context"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

// Tools returns the coding tools bound to ws.
// skillDirs are extra directories read_file may open. Writes stay inside ws.
func Tools(ws *Workspace, skillDirs []string) []agent.Tool {
	return []agent.Tool{
		readTool(ws, skillDirs),
		writeTool(ws),
		editTool(ws),
		listTool(ws),
		commandTool(ws),
	}
}

// newTool decodes JSON arguments once. The context is checked here so each
// tool body can start at its own check.
func newTool[A any](name, description string, params ai.Schema, run func(context.Context, A) (string, error)) agent.Tool {
	return agent.Tool{
		Name:        name,
		Description: description,
		Parameters:  params,
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			var args A
			if err := decodeArgs(raw, &args); err != nil {
				return "", err
			}
			return run(ctx, args)
		},
	}
}

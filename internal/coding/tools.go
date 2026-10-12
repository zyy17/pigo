package coding

import "github.com/zyy17/pigo/internal/agent"

// Tools returns the coding tools bound to ws.
func Tools(ws *Workspace) []agent.Tool {
	return []agent.Tool{
		readTool(ws),
		writeTool(ws),
		editTool(ws),
		listTool(ws),
		commandTool(ws),
	}
}

package coding

import "github.com/zyy17/pigo/internal/agent"

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

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

// render prints the events a person needs in order to follow the loop.
// Text deltas stream as they arrive. Tool calls print on their own lines.
func render(w io.Writer, ev agent.Event) {
	switch ev.Type {
	case agent.MessageUpdate:
		fmt.Fprint(w, ev.TextDelta)
	case agent.MessageEnd:
		if ev.Message.Role == ai.RoleAssistant && ev.Message.Text != "" && !strings.HasSuffix(ev.Message.Text, "\n") {
			fmt.Fprintln(w)
		}
		if line := usageLine(ev.Usage, ev.Session); line != "" {
			fmt.Fprintln(w, line)
		}
	case agent.Compacted:
		fmt.Fprintln(w, "compacted")
		if line := usageLine(ev.Usage, ev.Session); line != "" {
			fmt.Fprintln(w, line)
		}
	case agent.ToolStart:
		fmt.Fprintf(w, "→ %s %s\n", ev.ToolName, flattenLine(string(ev.Args), toolArgCols))
	case agent.ToolEnd:
		mark := "←"
		if ev.IsError {
			mark = "✗"
		}
		fmt.Fprintf(w, "%s %s\n", mark, flattenLine(ev.Result, toolResultCols))
	}
}

const (
	toolArgCols    = 160
	toolResultCols = 240
)

func flattenLine(s string, width int) string {
	return fitLine(oneLine(s), width)
}

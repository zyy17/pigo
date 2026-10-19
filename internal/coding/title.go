package coding

import (
	"encoding/json"
	"strings"
)

// RanTitle is the transcript line for a tool call. The command or the
// path tells the reader more than the tool name alone.
func RanTitle(name string, args []byte) string {
	var fields map[string]any
	_ = json.Unmarshal(args, &fields)
	if cmd, _ := fields["command"].(string); cmd != "" {
		return "• Ran " + oneArg(cmd)
	}
	if path, _ := fields["path"].(string); path != "" {
		return "• Ran " + name + " " + path
	}
	return "• Ran " + name
}

func oneArg(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

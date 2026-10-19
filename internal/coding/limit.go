package coding

// maxFileBytes is how much of a file a tool will read or edit.
// Larger files are refused so a single tool result cannot swallow the transcript.
const maxFileBytes = 100_000

// clipText keeps the tail. A compiler or test failure is usually at the end.
func clipText(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…(truncated)\n" + string(r[len(r)-max:])
}

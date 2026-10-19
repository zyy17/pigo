package cli

import (
	"strconv"
	"strings"

	"github.com/zyy17/pigo/internal/ai"
)

// tokenSegment is the Codex status-line phrasing for a token pair.
// Counts under 1000 stay exact. Larger counts use the same K/M/B/T scale Codex does.
func tokenSegment(in, out int) string {
	return compactTokens(in) + " in · " + compactTokens(out) + " out"
}

// usageLine is the one line -p and the line session print after a model call
// that reported usage. The session sum is added only when it differs.
func usageLine(call, session *ai.Usage) string {
	if call == nil {
		return ""
	}
	line := tokenSegment(call.Input, call.Output)
	if session == nil || (session.Input == call.Input && session.Output == call.Output) {
		return line
	}
	return line + " · session " + tokenSegment(session.Input, session.Output)
}

// compactTokens matches Codex format_tokens_compact.
func compactTokens(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return strconv.Itoa(n)
	}
	scaled := float64(n)
	suffix := "K"
	switch {
	case n >= 1_000_000_000_000:
		scaled /= 1_000_000_000_000
		suffix = "T"
	case n >= 1_000_000_000:
		scaled /= 1_000_000_000
		suffix = "B"
	case n >= 1_000_000:
		scaled /= 1_000_000
		suffix = "M"
	default:
		scaled /= 1000
	}
	decimals := 0
	if scaled < 10 {
		decimals = 2
	} else if scaled < 100 {
		decimals = 1
	}
	formatted := strconv.FormatFloat(scaled, 'f', decimals, 64)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(formatted, "0")
		formatted = strings.TrimRight(formatted, ".")
	}
	return formatted + suffix
}

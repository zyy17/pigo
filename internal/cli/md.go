package cli

import (
	"strings"
	"unicode/utf8"
)

// styleProse colors a wrapped assistant message. Body text keeps the
// terminal's foreground. Code, headings, and links are accented, and a
// unified diff is green or red.
func styleProse(text string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	code := false
	diff := false
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		var styled string
		styled, code, diff = styleProseLine(line, code, diff)
		b.WriteString(styled)
	}
	return b.String()
}

func styleProseLine(line string, code, diff bool) (string, bool, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "```") {
		return dimStyle.Render(line), !code, false
	}
	if code {
		return styleCodeLine(line), true, false
	}
	if isDiffHeader(line) {
		return dimStyle.Render(line), false, true
	}
	if diff {
		if trimmed == "" {
			return line, false, false
		}
		if styled, ok := styleDiffLine(line); ok {
			return styled, false, true
		}
		return styleTextLine(line), false, false
	}
	return styleTextLine(line), false, false
}

func styleCodeLine(line string) string {
	if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
		if styled, ok := styleDiffLine(line); ok {
			return styled
		}
	}
	return codeStyle.Render(line)
}

func isDiffHeader(line string) bool {
	switch {
	case strings.HasPrefix(line, "diff --git "),
		strings.HasPrefix(line, "@@"),
		strings.HasPrefix(line, "+++ "),
		strings.HasPrefix(line, "--- ") && strings.TrimSpace(line) != "---":
		return true
	default:
		return false
	}
}

func styleDiffLine(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return diffAddStyle.Render(line), true
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return diffDelStyle.Render(line), true
	case strings.HasPrefix(line, " "):
		return line, true
	default:
		return "", false
	}
}

func styleTextLine(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	if mark, rest, ok := splitHeading(trimmed); ok {
		return headingStyle.Render(indent + mark + rest)
	}
	if strings.HasPrefix(trimmed, "> ") || trimmed == ">" {
		return quoteStyle.Render(line)
	}
	if mark, rest, ok := splitList(trimmed); ok {
		return indent + listMarkStyle.Render(mark) + styleInline(rest)
	}
	return styleInline(line)
}

func splitHeading(line string) (mark, rest string, ok bool) {
	n := 0
	for n < len(line) && n < 6 && line[n] == '#' {
		n++
	}
	if n == 0 || n >= len(line) || line[n] != ' ' {
		return "", "", false
	}
	return line[:n+1], line[n+1:], true
}

func splitList(line string) (mark, rest string, ok bool) {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return line[:2], line[2:], true
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' ' {
		return line[:i+2], line[i+2:], true
	}
	return "", "", false
}

func styleInline(line string) string {
	var b strings.Builder
	for len(line) > 0 {
		switch {
		case strings.HasPrefix(line, "**"):
			if inner, rest, ok := cutMarked(line, "**"); ok {
				b.WriteString(strongStyle.Render(inner))
				line = rest
				continue
			}
		case strings.HasPrefix(line, "`"):
			if inner, rest, ok := cutMarked(line, "`"); ok {
				b.WriteString(codeStyle.Render(inner))
				line = rest
				continue
			}
		case strings.HasPrefix(line, "["):
			if label, url, rest, ok := cutLink(line); ok {
				b.WriteString(linkStyle.Render(label))
				b.WriteString(dimStyle.Render(" (" + url + ")"))
				line = rest
				continue
			}
		case strings.HasPrefix(line, "*"):
			if inner, rest, ok := cutMarked(line, "*"); ok {
				b.WriteString(emStyle.Render(inner))
				line = rest
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(line)
		b.WriteRune(r)
		line = line[size:]
	}
	return b.String()
}

func cutMarked(line, mark string) (inner, rest string, ok bool) {
	body := line[len(mark):]
	end := strings.Index(body, mark)
	if end <= 0 {
		return "", "", false
	}
	return body[:end], body[end+len(mark):], true
}

func cutLink(line string) (label, url, rest string, ok bool) {
	endLabel := strings.IndexByte(line, ']')
	if endLabel < 2 || endLabel+1 >= len(line) || line[endLabel+1] != '(' {
		return "", "", "", false
	}
	endURL := strings.IndexByte(line[endLabel+2:], ')')
	if endURL < 1 {
		return "", "", "", false
	}
	endURL += endLabel + 2
	return line[1:endLabel], line[endLabel+2 : endURL], line[endURL+1:], true
}

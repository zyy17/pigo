// Slash menu above the composer.
//
// Built-in commands and discovered skills share one list. Typing "/"
// opens it; Esc dismisses it until the next edit. Enter completes a
// partial name and sends an exact one.

package cli

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/skills"
)

const suggestWindow = 5

func slashCommands(list []skills.Skill) []slashCommand {
	cmds := append([]slashCommand(nil), builtins...)
	for _, skill := range list {
		desc := oneLine(skill.Description)
		if desc == "" {
			desc = "run this skill"
		}
		cmds = append(cmds, slashCommand{Name: "/skill:" + skill.Name, Desc: desc})
	}
	return cmds
}

func filterCommands(cmds []slashCommand, query string) []slashCommand {
	q := strings.ToLower(query)
	out := make([]slashCommand, 0, len(cmds))
	for _, cmd := range cmds {
		if strings.HasPrefix(strings.ToLower(cmd.Name), q) {
			out = append(out, cmd)
		}
	}
	return out
}

// partialCommand reports that Enter should fill in the selection instead of
// sending. An exact command, or text that already has an argument, sends.
func partialCommand(query string, matches []slashCommand) bool {
	if query == "" || strings.ContainsAny(query, " \n") || len(matches) == 0 {
		return false
	}
	for _, cmd := range matches {
		if strings.EqualFold(cmd.Name, query) {
			return false
		}
	}
	return true
}

func (m *chatModel) commandList() []slashCommand {
	return slashCommands(m.skills)
}

func (m *chatModel) matches() []slashCommand {
	return filterCommands(m.commandList(), m.comp.text())
}

func (m *chatModel) menuOpen() bool {
	if m.suggestDismissed || m.busy {
		return false
	}
	text := m.comp.text()
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \n") {
		return false
	}
	return len(m.matches()) > 0
}

func (m *chatModel) noteTyped() {
	m.suggestDismissed = false
	m.suggestAt = 0
}

func (m *chatModel) moveSuggest(delta int) {
	n := len(m.matches())
	if n == 0 {
		return
	}
	m.suggestAt = (m.suggestAt + delta) % n
	if m.suggestAt < 0 {
		m.suggestAt += n
	}
}

func (m *chatModel) completeSuggest() {
	items := m.matches()
	if len(items) == 0 {
		return
	}
	if m.suggestAt < 0 || m.suggestAt >= len(items) {
		m.suggestAt = 0
	}
	m.comp.set(items[m.suggestAt].Name)
	m.suggestAt = 0
}

func (m *chatModel) suggestView() string {
	if !m.menuOpen() {
		return ""
	}
	items := m.matches()
	start, end := suggestRange(len(items), m.suggestAt, suggestWindow)
	width := m.width
	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(paintSuggest(items[i], width, i == m.suggestAt))
	}
	return b.String()
}

func suggestRange(n, idx, size int) (start, end int) {
	if n <= size || size < 1 {
		return 0, n
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	start = idx - size/2
	if start < 0 {
		start = 0
	}
	end = start + size
	if end > n {
		end = n
		start = end - size
	}
	return start, end
}

func paintSuggest(cmd slashCommand, width int, selected bool) string {
	gap := "  "
	room := width - ansi.StringWidth(cmd.Name) - len(gap)
	line := cmd.Name
	if room > 0 && cmd.Desc != "" {
		line += gap + fitLine(cmd.Desc, room)
	} else {
		line = fitLine(line, width)
	}
	pad := width - ansi.StringWidth(line)
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	if selected {
		return suggestOnStyle.Render(line)
	}
	return suggestStyle.Render(line)
}

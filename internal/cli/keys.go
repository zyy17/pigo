package cli

import (
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// shiftEnterMsg is a newline. Bubble Tea v1 has no Shift+Enter key, so the
// program filter turns the terminal's modified-Enter sequence into this.
type shiftEnterMsg struct{}

// xterm modifyOtherKeys mode 1. Unmodified keys stay as they are, and
// Shift+Enter, which would otherwise be a plain CR, arrives as CSI 27 ; 2 ; 13 ~.
// Bubble Tea v2 can request the kitty keyboard protocol itself. v1 cannot, and
// turning that protocol on would re-encode keys this version does not read.
const (
	enableModifyOtherKeys  = "\x1b[>4;1m"
	disableModifyOtherKeys = "\x1b[>4;0m"
)

func enableShiftEnter(w io.Writer) {
	_, _ = io.WriteString(w, enableModifyOtherKeys)
}

func disableShiftEnter(w io.Writer) {
	_, _ = io.WriteString(w, disableModifyOtherKeys)
}

// rewriteKeys lets Shift+Enter through as a newline on terminals that report it.
func rewriteKeys(_ tea.Model, msg tea.Msg) tea.Msg {
	if isShiftEnter(msg) {
		return shiftEnterMsg{}
	}
	return msg
}

func isShiftEnter(msg tea.Msg) bool {
	seq, ok := sequenceBytes(msg)
	return ok && isShiftEnterCSI(seq)
}

// sequenceBytes reads the unrecognized CSI message Bubble Tea keeps unexported.
func sequenceBytes(msg tea.Msg) ([]byte, bool) {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Name() == "" || v.Type().Elem().Kind() != reflect.Uint8 {
		return nil, false
	}
	buf := make([]byte, v.Len())
	reflect.Copy(reflect.ValueOf(buf), v)
	return buf, true
}

// isShiftEnterCSI matches xterm modifyOtherKeys (CSI 27 ; mod ; 13 ~) and
// kitty's CSI-u form (CSI 13 ; mod u). The shift bit is the low bit of mod-1.
func isShiftEnterCSI(seq []byte) bool {
	s := string(seq)
	if rest, ok := strings.CutPrefix(s, "\x1b[27;"); ok && strings.HasSuffix(rest, "~") {
		parts := strings.Split(strings.TrimSuffix(rest, "~"), ";")
		return len(parts) >= 2 && parts[1] == "13" && shifted(parts[0])
	}
	if rest, ok := strings.CutPrefix(s, "\x1b[13;"); ok && strings.HasSuffix(rest, "u") {
		mod, _, _ := strings.Cut(strings.TrimSuffix(rest, "u"), ";")
		return shifted(mod)
	}
	return false
}

func shifted(mod string) bool {
	n, err := strconv.Atoi(mod)
	if err != nil || n < 2 {
		return false
	}
	return (n-1)&1 == 1
}

// continuedLine is the Codex-style fallback: a trailing backslash and Enter
// inserts a newline and drops that backslash. An even count is an escaped
// backslash, so the line submits as typed. Spaces after the slash cancel it.
func continuedLine(text string) (string, bool) {
	head := ""
	line := text
	if i := strings.LastIndex(text, "\n"); i >= 0 {
		head = text[:i+1]
		line = text[i+1:]
	}
	if strings.TrimRight(line, " \t") != line {
		return "", false
	}
	n := 0
	for strings.HasSuffix(line, `\`) {
		n++
		line = strings.TrimSuffix(line, `\`)
	}
	if n == 0 || n%2 == 0 {
		return "", false
	}
	return head + line + strings.Repeat(`\`, n-1) + "\n", true
}

func (m chatModel) newline() (tea.Model, tea.Cmd) {
	m.comp.insert("\n")
	m.noteTyped()
	return m, nil
}

func (m chatModel) onEnter() (tea.Model, tea.Cmd) {
	if m.menuOpen() && partialCommand(m.comp.text(), m.matches()) {
		m.completeSuggest()
		return m, nil
	}
	if next, ok := continuedLine(m.comp.text()); ok {
		m.comp.set(next)
		m.noteTyped()
		return m, nil
	}
	return m.submit(strings.TrimSpace(m.comp.text()))
}

// fmtElapsed is the compact clock on the working row: 3s, 1m 05s, 1h 02m 03s.
func fmtElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	if s < 60 {
		return strconv.Itoa(s) + "s"
	}
	if s < 3600 {
		return strconv.Itoa(s/60) + "m " + twoDigits(s%60) + "s"
	}
	return strconv.Itoa(s/3600) + "h " + twoDigits((s%3600)/60) + "m " + twoDigits(s%60) + "s"
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

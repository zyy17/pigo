package coding

import (
	"path/filepath"
	"strings"
	"unicode"
)

// riskyCommand reports why a shell command looks destructive.
// It is a short denylist for obvious slips. It is not a sandbox.
func riskyCommand(command string) (string, bool) {
	if strings.Contains(compact(command), ":(){:|:&};:") {
		return "fork bomb", true
	}
	for _, seg := range shellSegments(command) {
		if reason, ok := riskySegment(seg); ok {
			return reason, true
		}
	}
	return "", false
}

func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
}

func riskySegment(seg string) (string, bool) {
	if pipeToShell(seg) {
		return "pipe into a shell", true
	}
	if redirectToDisk(seg) {
		return "redirect to a disk device", true
	}
	name, args := commandWord(shellWords(seg))
	switch {
	case name == "sudo":
		return "sudo", true
	case name == "mkfs" || strings.HasPrefix(name, "mkfs."):
		return "mkfs", true
	case name == "dd" && ddToDisk(args):
		return "dd to a disk device", true
	case name == "git" && gitPushForce(args):
		return "git push --force", true
	case name == "chmod" && chmodRoot777(args):
		return "chmod -R 777 /", true
	case name == "rm" && rmRootOrHome(args):
		return "recursive delete of / or home", true
	default:
		return "", false
	}
}

func pipeToShell(seg string) bool {
	parts := splitOn(seg, pipeSplit)
	if len(parts) < 2 {
		return false
	}
	for i := range len(parts) - 1 {
		left, _ := commandWord(shellWords(parts[i]))
		if left != "curl" && left != "wget" {
			continue
		}
		right, rest := commandWord(shellWords(parts[i+1]))
		if isShell(right) || (right == "sudo" && sudoThenShell(rest)) {
			return true
		}
	}
	return false
}

func sudoThenShell(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return isShell(filepath.Base(arg))
	}
	return false
}

func isShell(name string) bool {
	switch name {
	case "sh", "bash", "zsh", "dash", "ksh":
		return true
	default:
		return false
	}
}

func ddToDisk(args []string) bool {
	for _, arg := range args {
		if target, ok := strings.CutPrefix(arg, "of="); ok && diskDevice(target) {
			return true
		}
	}
	return false
}

func gitPushForce(args []string) bool {
	push := -1
	for i, arg := range args {
		if arg == "push" {
			push = i
			break
		}
		if !strings.HasPrefix(arg, "-") {
			return false
		}
	}
	if push < 0 {
		return false
	}
	for _, arg := range args[push+1:] {
		if arg == "--force" || arg == "-f" {
			return true
		}
		if isShortCluster(arg) && strings.ContainsRune(arg, 'f') {
			return true
		}
	}
	return false
}

func chmodRoot777(args []string) bool {
	recursive := false
	mode := ""
	var paths []string
	for _, arg := range args {
		switch {
		case arg == "-R" || arg == "-r" || arg == "--recursive":
			recursive = true
		case isShortCluster(arg) && (strings.ContainsRune(arg, 'R') || strings.ContainsRune(arg, 'r')):
			recursive = true
		case mode == "" && isOctalMode(arg):
			mode = arg
		case arg != "--" && !strings.HasPrefix(arg, "-"):
			paths = append(paths, arg)
		}
	}
	if !recursive || (mode != "777" && mode != "0777") {
		return false
	}
	for _, path := range paths {
		if path == "/" || path == "//" || path == "/*" {
			return true
		}
	}
	return false
}

func rmRootOrHome(args []string) bool {
	recursive, force := false, false
	var paths []string
	operands := false
	for _, arg := range args {
		switch {
		case arg == "--":
			operands = true
		case !operands && arg == "--recursive":
			recursive = true
		case !operands && arg == "--force":
			force = true
		case !operands && isShortCluster(arg):
			if strings.ContainsRune(arg, 'r') || strings.ContainsRune(arg, 'R') {
				recursive = true
			}
			if strings.ContainsRune(arg, 'f') {
				force = true
			}
		case !operands && strings.HasPrefix(arg, "-"):
		default:
			operands = true
			paths = append(paths, arg)
		}
	}
	if !recursive || !force {
		return false
	}
	for _, path := range paths {
		if isRootOrHome(path) {
			return true
		}
	}
	return false
}

func isRootOrHome(path string) bool {
	switch path {
	case "/", "//", "/*", "/.", "~", "~/", "~/*",
		"$HOME", "$HOME/", "$HOME/*",
		"${HOME}", "${HOME}/", "${HOME}/*":
		return true
	default:
		return false
	}
}

func redirectToDisk(segment string) bool {
	r := []rune(segment)
	inSingle, inDouble := false, false
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case inSingle:
			inSingle = c != '\''
		case inDouble:
			if c == '\\' && i+1 < len(r) {
				i++
				continue
			}
			inDouble = c != '"'
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '\\' && i+1 < len(r):
			i++
		case c == '>':
			for i+1 < len(r) && r[i+1] == '>' {
				i++
			}
			if diskDevice(nextWord(string(r[i+1:]))) {
				return true
			}
		}
	}
	return false
}

func nextWord(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s == "" {
		return ""
	}
	end := strings.IndexFunc(s, unicode.IsSpace)
	if end < 0 {
		end = len(s)
	}
	return strings.Trim(s[:end], `"'`)
}

func diskDevice(path string) bool {
	name, ok := strings.CutPrefix(path, "/dev/")
	if !ok || name == "" {
		return false
	}
	for _, stem := range []string{"sd", "nvme", "hd", "mmcblk", "xvd", "vd"} {
		if strings.HasPrefix(name, stem) {
			return true
		}
	}
	return false
}

func commandWord(words []string) (string, []string) {
	i := 0
	for i < len(words) && isEnvAssign(words[i]) {
		i++
	}
	if i >= len(words) {
		return "", nil
	}
	return filepath.Base(words[i]), words[i+1:]
}

func isEnvAssign(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || unicode.IsLetter(r) {
			continue
		}
		if i > 0 && unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func isShortCluster(arg string) bool {
	return len(arg) > 1 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--")
}

func isOctalMode(arg string) bool {
	if len(arg) < 3 || len(arg) > 4 {
		return false
	}
	for _, r := range arg {
		if r < '0' || r > '7' {
			return false
		}
	}
	return true
}

func shellWords(s string) []string {
	var words []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		words = append(words, b.String())
		b.Reset()
	}
	walkShell(s, func(c rune, quoted bool) {
		if !quoted && (c == ' ' || c == '\t') {
			flush()
			return
		}
		b.WriteRune(c)
	})
	flush()
	return words
}

func shellSegments(command string) []string {
	return splitOn(command, segmentSplit)
}

func pipeSplit(c, next rune) bool {
	return c == '|' && next != '|'
}

func segmentSplit(c, next rune) bool {
	switch c {
	case ';', '\n':
		return true
	case '&':
		return next == '&'
	case '|':
		return next == '|'
	default:
		return false
	}
}

func splitOn(s string, at func(c, next rune) bool) []string {
	var parts []string
	var b strings.Builder
	flush := func() {
		parts = append(parts, b.String())
		b.Reset()
	}
	r := []rune(s)
	inSingle, inDouble := false, false
	for i := 0; i < len(r); i++ {
		c := r[i]
		next := rune(0)
		if i+1 < len(r) {
			next = r[i+1]
		}
		switch {
		case inSingle:
			b.WriteRune(c)
			inSingle = c != '\''
		case inDouble:
			b.WriteRune(c)
			if c == '\\' && i+1 < len(r) {
				i++
				b.WriteRune(r[i])
				continue
			}
			inDouble = c != '"'
		case c == '\'':
			inSingle = true
			b.WriteRune(c)
		case c == '"':
			inDouble = true
			b.WriteRune(c)
		case c == '\\' && i+1 < len(r):
			b.WriteRune(c)
			i++
			b.WriteRune(r[i])
		case at(c, next):
			flush()
			if (c == '&' && next == '&') || (c == '|' && next == '|') {
				i++
			}
		default:
			b.WriteRune(c)
		}
	}
	flush()
	return parts
}

// walkShell visits each rune of a shell word list. Quotes are stripped.
// A backslash outside quotes keeps the next rune unquoted.
func walkShell(s string, visit func(c rune, quoted bool)) {
	r := []rune(s)
	inSingle, inDouble := false, false
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
				continue
			}
			visit(c, true)
		case inDouble:
			if c == '\\' && i+1 < len(r) {
				i++
				visit(r[i], true)
				continue
			}
			if c == '"' {
				inDouble = false
				continue
			}
			visit(c, true)
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '\\' && i+1 < len(r):
			i++
			visit(r[i], true)
		default:
			visit(c, false)
		}
	}
}

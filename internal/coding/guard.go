package coding

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

// riskyCommand reports why a shell command looks destructive.
// It is a short denylist for obvious slips. It is not a sandbox.
func riskyCommand(command string) (string, bool) {
	if strings.Contains(compact(command), ":(){:|:&};:") {
		return "fork bomb", true
	}
	for _, seg := range split(tokens(command), ";", "\n", "&&", "||") {
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

func riskySegment(toks []string) (string, bool) {
	if pipeToShell(toks) {
		return "pipe into a shell", true
	}
	if redirectToDisk(toks) {
		return "redirect to a disk device", true
	}
	name, args := commandWord(toks)
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

func pipeToShell(toks []string) bool {
	parts := split(toks, "|")
	if len(parts) < 2 {
		return false
	}
	for i := range len(parts) - 1 {
		left, _ := commandWord(parts[i])
		if left != "curl" && left != "wget" {
			continue
		}
		right, rest := commandWord(parts[i+1])
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
	rest, ok := afterSubcommand(args, "push")
	if !ok {
		return false
	}
	flags, long, _ := parseArgs(rest, false)
	return strings.ContainsRune(flags, 'f') || slices.Contains(long, "--force")
}

// afterSubcommand is the words after name. A non-option before name means
// this is not that subcommand. git -C dir push is the case that returns false.
func afterSubcommand(args []string, name string) ([]string, bool) {
	for i, arg := range args {
		if arg == name {
			return args[i+1:], true
		}
		if !strings.HasPrefix(arg, "-") {
			return nil, false
		}
	}
	return nil, false
}

func chmodRoot777(args []string) bool {
	flags, long, operands := parseArgs(args, false)
	if !strings.ContainsAny(flags, "rR") && !slices.Contains(long, "--recursive") {
		return false
	}
	mode := ""
	for _, arg := range operands {
		if mode == "" && isOctalMode(arg) {
			mode = arg
		}
	}
	if mode != "777" && mode != "0777" {
		return false
	}
	for _, path := range operands {
		if path == "/" || path == "//" || path == "/*" {
			return true
		}
	}
	return false
}

func rmRootOrHome(args []string) bool {
	flags, long, paths := parseArgs(args, true)
	recursive := strings.ContainsAny(flags, "rR") || slices.Contains(long, "--recursive")
	force := strings.ContainsRune(flags, 'f') || slices.Contains(long, "--force")
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

func redirectToDisk(toks []string) bool {
	for i, tok := range toks {
		if tok != ">" || i+1 >= len(toks) {
			continue
		}
		if diskDevice(toks[i+1]) {
			return true
		}
	}
	return false
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

// tokens splits a command into words and operators. Quotes are stripped.
// A backslash outside quotes, and a backslash inside double quotes, keeps
// the next rune as part of the word.
func tokens(command string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	r := []rune(command)
	inSingle, inDouble := false, false
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
				continue
			}
			b.WriteRune(c)
		case inDouble:
			if c == '\\' && i+1 < len(r) {
				i++
				b.WriteRune(r[i])
				continue
			}
			if c == '"' {
				inDouble = false
				continue
			}
			b.WriteRune(c)
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '\\' && i+1 < len(r):
			i++
			b.WriteRune(r[i])
		case c == ' ' || c == '\t':
			flush()
		case c == '\n' || c == ';':
			flush()
			out = append(out, string(c))
		case c == '&' && i+1 < len(r) && r[i+1] == '&':
			flush()
			i++
			out = append(out, "&&")
		case c == '|' && i+1 < len(r) && r[i+1] == '|':
			flush()
			i++
			out = append(out, "||")
		case c == '|':
			flush()
			out = append(out, "|")
		case c == '>':
			flush()
			for i+1 < len(r) && r[i+1] == '>' {
				i++
			}
			out = append(out, ">")
		default:
			b.WriteRune(c)
		}
	}
	flush()
	return out
}

// split breaks toks on any separator in seps. The separators are dropped.
func split(toks []string, seps ...string) [][]string {
	var parts [][]string
	var cur []string
	for _, tok := range toks {
		if slices.Contains(seps, tok) {
			parts = append(parts, cur)
			cur = nil
			continue
		}
		cur = append(cur, tok)
	}
	return append(parts, cur)
}

// parseArgs splits short-flag letters, long options, and operands.
// "--" ends options and is not an operand. stop makes the first operand
// end them too, which is how rm treats a later -rf as a filename.
// chmod and git keep reading options after a path.
func parseArgs(args []string, stop bool) (flags string, long, operands []string) {
	opt := true
	for _, arg := range args {
		switch {
		case arg == "--" && opt:
			opt = false
		case opt && strings.HasPrefix(arg, "--"):
			long = append(long, arg)
		case opt && isShortCluster(arg):
			flags += arg[1:]
		default:
			operands = append(operands, arg)
			if stop {
				opt = false
			}
		}
	}
	return flags, long, operands
}

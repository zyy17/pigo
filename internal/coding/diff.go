package coding

import (
	"fmt"
	"path/filepath"
	"strings"
)

// maxDiffLines bounds the line-diff table. A longer file gets a short
// preview so the review stays readable.
const maxDiffLines = 800

// diffContext is how many unchanged lines stay around a change.
const diffContext = 3

// maxDiffRunes caps the text handed to the confirm panel.
const maxDiffRunes = 8_000

const previewHead = 12

type opKind int

const (
	opSame opKind = iota
	opDel
	opIns
)

type diffOp struct {
	kind opKind
	text string
}

type hunkSpan struct{ start, end int }

// unifiedDiff is a unified diff of before and after.
// existed is false when the path is not a file yet, so the old side is /dev/null.
// An empty result means the lines and the trailing newline already match.
func unifiedDiff(path, before, after string, existed bool) string {
	oldLines, oldNL := splitLines(before)
	newLines, newNL := splitLines(after)
	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return clipDiff(longPreview(path, oldLines, newLines))
	}
	ops := diffLines(oldLines, newLines)
	if !hasEdit(ops) && oldNL == newNL {
		return ""
	}
	// A trailing-newline change is invisible to the line table. Show the
	// last line as a replacement so the marker has somewhere to sit.
	if !hasEdit(ops) && len(ops) > 0 {
		last := ops[len(ops)-1].text
		ops = ops[:len(ops)-1]
		ops = append(ops, diffOp{kind: opDel, text: last}, diffOp{kind: opIns, text: last})
	}
	oldHdr, newHdr := diffHeaders(path, existed)
	return clipDiff(formatUnified(oldHdr, newHdr, ops, oldNL, newNL))
}

func diffHeaders(path string, existed bool) (string, string) {
	next := "+++ " + diffName("b", path)
	if !existed {
		return "--- /dev/null", next
	}
	return "--- " + diffName("a", path), next
}

func diffName(prefix, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return prefix + "/" + path
}

func splitLines(s string) ([]string, bool) {
	if s == "" {
		return nil, false
	}
	nl := strings.HasSuffix(s, "\n")
	if nl {
		s = strings.TrimSuffix(s, "\n")
	}
	return strings.Split(s, "\n"), nl
}

// diffLines is the shortest edit from a to b.
// The table is the length of the common tail from each line.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	stride := m + 1
	dp := make([]int, (n+1)*stride)
	at := func(i, j int) int { return dp[i*stride+j] }

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i*stride+j] = at(i+1, j+1) + 1
				continue
			}
			del, ins := at(i+1, j), at(i, j+1)
			if del >= ins {
				dp[i*stride+j] = del
			} else {
				dp[i*stride+j] = ins
			}
		}
	}

	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{kind: opSame, text: a[i]})
			i++
			j++
		case at(i+1, j) >= at(i, j+1):
			ops = append(ops, diffOp{kind: opDel, text: a[i]})
			i++
		default:
			ops = append(ops, diffOp{kind: opIns, text: b[j]})
			j++
		}
	}
	for i < n {
		ops = append(ops, diffOp{kind: opDel, text: a[i]})
		i++
	}
	for j < m {
		ops = append(ops, diffOp{kind: opIns, text: b[j]})
		j++
	}
	return ops
}

func hasEdit(ops []diffOp) bool {
	for _, op := range ops {
		if op.kind != opSame {
			return true
		}
	}
	return false
}

func formatUnified(oldHdr, newHdr string, ops []diffOp, oldNL, newNL bool) string {
	var b strings.Builder
	b.WriteString(oldHdr)
	b.WriteByte('\n')
	b.WriteString(newHdr)
	b.WriteByte('\n')
	for _, h := range collectHunks(ops) {
		writeHunk(&b, ops, h, oldNL, newNL)
	}
	return b.String()
}

func collectHunks(ops []diffOp) []hunkSpan {
	n := len(ops)
	show := make([]bool, n)
	for i, op := range ops {
		if op.kind == opSame {
			continue
		}
		lo := max(i-diffContext, 0)
		hi := min(i+diffContext+1, n)
		for j := lo; j < hi; j++ {
			show[j] = true
		}
	}
	var hunks []hunkSpan
	for i := 0; i < n; {
		if !show[i] {
			i++
			continue
		}
		j := i + 1
		for j < n && show[j] {
			j++
		}
		hunks = append(hunks, hunkSpan{i, j})
		i = j
	}
	return hunks
}

func writeHunk(b *strings.Builder, ops []diffOp, h hunkSpan, oldNL, newNL bool) {
	oldStart, newStart := lineStarts(ops, h.start)
	oldCount, newCount := 0, 0
	lines := make([]string, 0, h.end-h.start)
	for i := h.start; i < h.end; i++ {
		op := ops[i]
		prefix := " "
		switch op.kind {
		case opDel:
			prefix = "-"
			oldCount++
		case opIns:
			prefix = "+"
			newCount++
		default:
			oldCount++
			newCount++
		}
		lines = append(lines, prefix+op.text)
		if missingNewline(ops, i, oldNL, newNL) {
			lines = append(lines, `\ No newline at end of file`)
		}
	}
	if oldCount == 0 {
		oldStart = 0
	}
	if newCount == 0 {
		newStart = 0
	}
	fmt.Fprintf(b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func lineStarts(ops []diffOp, end int) (oldStart, newStart int) {
	oldStart, newStart = 1, 1
	for _, op := range ops[:end] {
		switch op.kind {
		case opDel:
			oldStart++
		case opIns:
			newStart++
		default:
			oldStart++
			newStart++
		}
	}
	return oldStart, newStart
}

func missingNewline(ops []diffOp, i int, oldNL, newNL bool) bool {
	op := ops[i]
	old := op.kind != opIns && !oldNL && lastOf(ops, i, func(k opKind) bool {
		return k == opSame || k == opDel
	})
	newer := op.kind != opDel && !newNL && lastOf(ops, i, func(k opKind) bool {
		return k == opSame || k == opIns
	})
	return old || newer
}

func lastOf(ops []diffOp, i int, keep func(opKind) bool) bool {
	if !keep(ops[i].kind) {
		return false
	}
	for _, op := range ops[i+1:] {
		if keep(op.kind) {
			return false
		}
	}
	return true
}

func longPreview(path string, oldLines, newLines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is too long for a line diff (%d → %d lines)\n", path, len(oldLines), len(newLines))
	writeHead(&b, "before", oldLines)
	writeHead(&b, "after", newLines)
	return b.String()
}

func writeHead(b *strings.Builder, label string, lines []string) {
	fmt.Fprintf(b, "%s:\n", label)
	n := min(len(lines), previewHead)
	for i := range n {
		b.WriteString("  ")
		b.WriteString(lines[i])
		b.WriteByte('\n')
	}
	if extra := len(lines) - n; extra > 0 {
		fmt.Fprintf(b, "  … %d more lines\n", extra)
	}
}

func clipDiff(s string) string {
	r := []rune(s)
	if len(r) <= maxDiffRunes {
		return s
	}
	return string(r[:maxDiffRunes]) + "\n… diff truncated\n"
}

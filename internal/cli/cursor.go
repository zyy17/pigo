package cli

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// frameCursor is where the real terminal cursor should sit after a frame.
// x is the column. up is how many lines above the last view line that is.
// Bubble Tea parks the hardware cursor on the last line, which is where an
// IME popup would open, so we move it onto the caret after each frame.
type frameCursor struct {
	mu   sync.Mutex
	x    int
	up   int
	show bool
}

func (f *frameCursor) set(x, up int, show bool) {
	if x < 0 {
		x = 0
	}
	if up < 0 {
		up = 0
	}
	f.mu.Lock()
	f.x, f.up, f.show = x, up, show
	f.mu.Unlock()
}

func (f *frameCursor) get() (x, up int, show bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.x, f.up, f.show
}

// placingWriter lets Bubble Tea draw a frame, then moves the real cursor
// onto the caret. Bubble Tea v1 has no cursor-position API, and it hides
// the cursor for the whole program.
//
// An inline frame ends in CR with the cursor on the last line. The next
// inline frame expects it back there, so we step down before that write.
// An alt-screen frame ends in CUP to that same cell (ESC [ row ; H) and
// the next frame homes itself. Stepping down first would show the caret
// at the bottom-left, so that path only hides the cursor while the frame
// paints.
// file, when set, keeps the writer a terminal so window size and raw mode work.
type placingWriter struct {
	out  io.Writer
	file *os.File
	cur  *frameCursor

	mu       sync.Mutex
	parked   bool
	parkedUp int
}

func (w *placingWriter) Fd() uintptr {
	if w.file != nil {
		return w.file.Fd()
	}
	return 0
}

func (w *placingWriter) Read(p []byte) (int, error) {
	if w.file != nil {
		return w.file.Read(p)
	}
	return 0, io.EOF
}

func (w *placingWriter) Close() error { return nil }

func (w *placingWriter) dest() io.Writer {
	if w.out != nil {
		return w.out
	}
	return w.file
}

func (w *placingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	kind := frameKind(p)
	if kind != frameNone && w.parked {
		restore := ansi.HideCursor
		if kind == frameInline {
			if w.parkedUp > 0 {
				restore += ansi.CursorDown(w.parkedUp)
			}
			restore += "\r"
		}
		if _, err := io.WriteString(w.dest(), restore); err != nil {
			return 0, err
		}
		w.parked = false
		w.parkedUp = 0
	}

	n, err := w.dest().Write(p)
	if err != nil || kind == frameNone || w.cur == nil {
		return n, err
	}
	x, up, show := w.cur.get()
	if !show {
		return n, err
	}
	seq := ""
	if up > 0 {
		seq += ansi.CursorUp(up)
	}
	if x > 0 {
		seq += ansi.CursorForward(x)
	}
	// A steady bar sits in the gap before the next cell, which is where the
	// IME composition window should anchor.
	seq += ansi.SetCursorStyle(6) + ansi.ShowCursor
	if _, werr := io.WriteString(w.dest(), seq); werr != nil {
		return n, werr
	}
	w.parked = true
	w.parkedUp = up
	return n, nil
}

const (
	frameNone = iota
	frameInline
	frameAlt
)

// frameKind reports how Bubble Tea v1 ended this write.
// Inline renders finish with CR. Alt-screen renders finish with CUP to
// column 1 of the last view line: ESC [ <row> ; H, or ESC [ H for one line.
func frameKind(p []byte) int {
	if len(p) > 1 && p[len(p)-1] == '\r' {
		return frameInline
	}
	if altScreenFrame(p) {
		return frameAlt
	}
	return frameNone
}

func altScreenFrame(p []byte) bool {
	// A bare home is how entering the alt screen parks the cursor.
	if bytes.HasSuffix(p, []byte("\x1b[H")) && len(p) > len("\x1b[H") {
		return true
	}
	// ESC [ <row> ; H
	if len(p) < len("\x1b[1;H") || !bytes.HasSuffix(p, []byte(";H")) {
		return false
	}
	end := len(p) - 2
	start := end
	for start > 0 && p[start-1] >= '0' && p[start-1] <= '9' {
		start--
	}
	if start == end || start < 2 || p[start-2] != 0x1b || p[start-1] != '[' {
		return false
	}
	n, err := strconv.Atoi(string(p[start:end]))
	return err == nil && n > 1
}

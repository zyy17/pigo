package cli

import (
	"io"
	"os"
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
// onto the caret. The next frame starts by returning the cursor to the last
// line, which is where the renderer expects it.
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

	frame := len(p) > 1 && p[len(p)-1] == '\r'
	if frame && w.parked {
		restore := ansi.HideCursor
		if w.parkedUp > 0 {
			restore += ansi.CursorDown(w.parkedUp)
		}
		restore += "\r"
		if _, err := io.WriteString(w.dest(), restore); err != nil {
			return 0, err
		}
		w.parked = false
		w.parkedUp = 0
	}

	n, err := w.dest().Write(p)
	if err != nil || !frame || w.cur == nil {
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

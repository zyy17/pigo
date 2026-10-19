package ai

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
)

// A single SSE data line can hold a whole tool-call argument.
const (
	sseLineStart = 64 * 1024
	sseLineMax   = 1024 * 1024
)

// parseSSE reads a chat-completions event stream and sends ai events.
// It always ends with done or error, then returns. The caller closes out.
func parseSSE(ctx context.Context, r io.Reader, out chan<- Event) {
	acc := &accumulator{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, sseLineStart), sseLineMax)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			sendStreamError(out, err, acc.message())
			return
		}
		data, ok := dataField(sc.Text())
		if !ok {
			continue
		}
		if data == "[DONE]" {
			out <- Event{Type: EventDone, Message: acc.message()}
			return
		}
		if err := acc.consume(data, out); err != nil {
			sendStreamError(out, err, acc.message())
			return
		}
	}
	if err := sc.Err(); err != nil {
		sendStreamError(out, err, acc.message())
		return
	}
	if acc.finish != "" {
		out <- Event{Type: EventDone, Message: acc.message()}
		return
	}
	sendStreamError(out, errors.New("stream ended before the model finished"), acc.message())
}

func dataField(line string) (string, bool) {
	line = strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(line, "data:")
	if line == "" || strings.HasPrefix(line, ":") || !ok {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func sendStreamError(out chan<- Event, err error, partial Message) {
	out <- Event{Type: EventError, Err: err, Message: partial}
}

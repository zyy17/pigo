package ai

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
)

// parseSSE reads a chat-completions event stream and sends ai events.
// It always ends with done or error, then returns. The caller closes out.
func parseSSE(ctx context.Context, r io.Reader, out chan<- Event) {
	acc := &accumulator{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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
	if acc.done {
		out <- Event{Type: EventDone, Message: acc.message()}
		return
	}
	sendStreamError(out, errors.New("stream ended before the model finished"), acc.message())
}

func dataField(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

func sendStreamError(out chan<- Event, err error, partial Message) {
	out <- Event{Type: EventError, Err: err, Message: partial}
}

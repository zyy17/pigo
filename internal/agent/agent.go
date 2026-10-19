package agent

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/zyy17/pigo/internal/ai"
)

const defaultMaxTurns = 25

// ErrInterrupted means Abort or a canceled context stopped the run.
var ErrInterrupted = errors.New("interrupted")

// Config builds an Agent. Stream is required.
type Config struct {
	SystemPrompt string
	Model        string
	Tools        []Tool
	Stream       ai.StreamFunc
	MaxTurns     int
	Before       BeforeToolCall
	// Record runs after each transcript message is stored. The coding
	// session uses it to append a session file. Nil keeps the transcript
	// in memory only. SetMessages does not call Record.
	Record func(ai.Message) error
}

// Agent runs the tool loop. Prompt is not safe for concurrent use.
// Abort is safe to call from another goroutine.
type Agent struct {
	state    State
	stream   ai.StreamFunc
	maxTurns int
	before   BeforeToolCall
	record   func(ai.Message) error
	subs     []func(Event)
	usage    usageTotals

	mu     sync.Mutex
	cancel context.CancelFunc
}

type usageTotals struct {
	last   ai.Usage
	sum    ai.Usage
	lastOK bool
	sumOK  bool
}

// New checks the config and returns an idle agent.
func New(cfg Config) (*Agent, error) {
	if cfg.Stream == nil {
		return nil, errors.New("stream function is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("model is required")
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = defaultMaxTurns
	}
	return &Agent{
		state: State{
			SystemPrompt: cfg.SystemPrompt,
			Model:        cfg.Model,
			Tools:        append([]Tool(nil), cfg.Tools...),
			Messages:     nil,
		},
		stream:   cfg.Stream,
		maxTurns: cfg.MaxTurns,
		before:   cfg.Before,
		record:   cfg.Record,
	}, nil
}

// Subscribe registers a listener. Listeners run inline, in order.
func (a *Agent) Subscribe(fn func(Event)) {
	if fn == nil {
		return
	}
	a.mu.Lock()
	a.subs = append(a.subs, fn)
	a.mu.Unlock()
}

// Messages returns a copy of the transcript.
func (a *Agent) Messages() []ai.Message {
	return a.state.snapshot()
}

// SetMessages replaces the transcript. Resume uses it to install a session
// file that is already on disk, so Record is not called.
func (a *Agent) SetMessages(msgs []ai.Message) {
	a.state.Messages = append([]ai.Message(nil), msgs...)
}

// Reset drops the transcript and the token totals.
// Totals exist only in this process, so they go away with the messages.
func (a *Agent) Reset() {
	a.state.Messages = nil
	a.usage = usageTotals{}
}

// Usage returns the latest model call and the session sum.
func (a *Agent) Usage() Usage {
	return Usage{
		Last:        a.usage.last,
		Session:     a.usage.sum,
		HaveLast:    a.usage.lastOK,
		HaveSession: a.usage.sumOK,
	}
}

// noteCallUsage adds one reported call into the session sum.
// A nil call is an omitted object, not a zero, so the sum stays unchanged.
func (a *Agent) noteCallUsage(call *ai.Usage) (last, session *ai.Usage) {
	if call == nil {
		a.usage.last = ai.Usage{}
		a.usage.lastOK = false
	} else {
		a.usage.last = *call
		a.usage.lastOK = true
		a.usage.sum = a.usage.sum.Add(*call)
		a.usage.sumOK = true
	}
	return a.usage.snapshot()
}

func (u usageTotals) snapshot() (last, session *ai.Usage) {
	if u.lastOK {
		c := u.last
		last = &c
	}
	if u.sumOK {
		c := u.sum
		session = &c
	}
	return last, session
}

// Abort cancels the Prompt that is currently running.
func (a *Agent) Abort() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *Agent) setCancel(cancel context.CancelFunc) {
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
}

func (a *Agent) clearCancel() {
	a.mu.Lock()
	a.cancel = nil
	a.mu.Unlock()
}

// add stores one transcript message, then asks Record to persist it.
// The message stays in memory when Record fails, so this process can keep
// going. The file stops at the last line that was written.
func (a *Agent) add(msg ai.Message) error {
	a.state.Messages = append(a.state.Messages, msg)
	if a.record == nil {
		return nil
	}
	return a.record(msg)
}

func (a *Agent) emit(ev Event) {
	a.mu.Lock()
	subs := make([]func(Event), len(a.subs))
	copy(subs, a.subs)
	a.mu.Unlock()
	for _, fn := range subs {
		fn(ev)
	}
}

// request builds one model call. The system prompt is attached here and is
// not copied into the stored transcript.
func (a *Agent) request() ai.Request {
	return ai.Request{
		Model:    a.state.Model,
		Messages: ai.WithSystem(a.state.SystemPrompt, a.state.snapshot()),
		Tools:    a.state.schemas(),
	}
}

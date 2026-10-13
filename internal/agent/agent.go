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
}

// Agent runs the tool loop. Prompt is not safe for concurrent use.
// Abort is safe to call from another goroutine.
type Agent struct {
	state    State
	stream   ai.StreamFunc
	maxTurns int
	subs     []func(Event)

	mu     sync.Mutex
	cancel context.CancelFunc
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

// Reset drops the transcript and keeps the prompt, model, and tools.
func (a *Agent) Reset() {
	a.state.Messages = nil
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

func (a *Agent) add(msg ai.Message) {
	a.state.Messages = append(a.state.Messages, msg)
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

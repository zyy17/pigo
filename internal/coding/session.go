package coding

import (
	"context"
	"errors"
	"strings"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-4o-mini"
)

// Config selects the model endpoint and the workspace.
// Stream, when set, replaces the HTTP client. Tests use it.
type Config struct {
	APIKey    string
	BaseURL   string
	Model     string
	Workspace string
	MaxTurns  int
	Stream    ai.StreamFunc
}

// Session is one coding-agent conversation in a workspace.
type Session struct {
	agent *agent.Agent
	root  string
	model string
}

// New opens the workspace and builds the agent.
func New(cfg Config) (*Session, error) {
	cfg = cfg.applyDefaults()
	ws, err := Open(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	stream, err := cfg.stream()
	if err != nil {
		return nil, err
	}
	ag, err := agent.New(agent.Config{
		SystemPrompt: buildSystemPrompt(ws.Root()),
		Model:        cfg.Model,
		Tools:        Tools(ws),
		Stream:       stream,
		MaxTurns:     cfg.MaxTurns,
	})
	if err != nil {
		return nil, err
	}
	return &Session{agent: ag, root: ws.Root(), model: cfg.Model}, nil
}

func (c Config) applyDefaults() Config {
	if strings.TrimSpace(c.BaseURL) == "" {
		c.BaseURL = defaultBaseURL
	}
	if strings.TrimSpace(c.Model) == "" {
		c.Model = defaultModel
	}
	if strings.TrimSpace(c.Workspace) == "" {
		c.Workspace = "."
	}
	return c
}

func (c Config) stream() (ai.StreamFunc, error) {
	if c.Stream != nil {
		return c.Stream, nil
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("API key is not set")
	}
	client := &ai.Client{BaseURL: c.BaseURL, APIKey: c.APIKey}
	return client.Stream, nil
}

// Root is the workspace directory.
func (s *Session) Root() string { return s.root }

// Model is the model name sent on each request.
func (s *Session) Model() string { return s.model }

// Subscribe registers a listener for agent events.
func (s *Session) Subscribe(fn func(agent.Event)) { s.agent.Subscribe(fn) }

// Prompt runs one task in this session. The transcript is kept for the next call.
func (s *Session) Prompt(ctx context.Context, text string) error {
	return s.agent.Prompt(ctx, text)
}

// Reset clears the transcript.
func (s *Session) Reset() { s.agent.Reset() }

// Abort cancels the running prompt.
func (s *Session) Abort() { s.agent.Abort() }

package coding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/skills"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
	defaultModel   = "gpt-5.6-luna"
)

// Config selects the model endpoint and the workspace.
// Stream, when set, replaces the HTTP client. Tests use it.
// UserSkillsDir overrides ~/.pi/agent/skills. An empty value uses that default.
type Config struct {
	APIKey        string
	BaseURL       string
	Model         string
	Workspace     string
	UserSkillsDir string
	MaxTurns      int
	Stream        ai.StreamFunc
	// Yolo skips the dangerous-command check.
	Yolo bool
	// Confirm, when set, is asked before a matched command runs.
	// A nil Confirm denies the command.
	Confirm func(ctx context.Context, command, reason string) (bool, error)
}

// Session is one coding-agent conversation in a workspace.
type Session struct {
	agent   *agent.Agent
	root    string
	model   string
	skills  []skills.Skill
	yolo    bool
	confirm func(ctx context.Context, command, reason string) (bool, error)
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
	userDir := cfg.UserSkillsDir
	if strings.TrimSpace(userDir) == "" {
		userDir = skills.DefaultUserDir()
	}
	found := skills.Discover(ws.Root(), userDir)
	// The hook closes over this session so Confirm can be set after New.
	session := &Session{
		root:    ws.Root(),
		model:   cfg.Model,
		skills:  found,
		yolo:    cfg.Yolo,
		confirm: cfg.Confirm,
	}
	ag, err := agent.New(agent.Config{
		SystemPrompt: buildSystemPrompt(ws.Root(), found),
		Model:        cfg.Model,
		Tools:        Tools(ws, skillDirs(found)),
		Stream:       stream,
		MaxTurns:     cfg.MaxTurns,
		Before:       session.beforeTool,
	})
	if err != nil {
		return nil, err
	}
	session.agent = ag
	return session, nil
}

// SetConfirm installs the interactive check. Call it before Prompt.
// A nil confirm denies dangerous commands. The TUI is the only caller.
func (s *Session) SetConfirm(fn func(ctx context.Context, command, reason string) (bool, error)) {
	s.confirm = fn
}

func (s *Session) beforeTool(ctx context.Context, call ai.ToolCall) (agent.Decision, error) {
	if s.yolo || call.Name != runCommandTool {
		return agent.Decision{}, nil
	}
	var args commandArgs
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return agent.Decision{}, nil
	}
	reason, dangerous := riskyCommand(args.Command)
	if !dangerous {
		return agent.Decision{}, nil
	}
	if s.confirm == nil {
		return agent.Decision{Block: true, Reason: reason}, nil
	}
	allow, err := s.confirm(ctx, args.Command, reason)
	if err != nil {
		return agent.Decision{}, err
	}
	if !allow {
		return agent.Decision{Block: true, Reason: reason}, nil
	}
	return agent.Decision{}, nil
}

func skillDirs(list []skills.Skill) []string {
	seen := map[string]bool{}
	dirs := make([]string, 0, len(list))
	for _, skill := range list {
		if skill.BaseDir == "" || seen[skill.BaseDir] {
			continue
		}
		seen[skill.BaseDir] = true
		dirs = append(dirs, skill.BaseDir)
	}
	return dirs
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

// Skills returns the discovered skills in menu order.
func (s *Session) Skills() []skills.Skill {
	out := make([]skills.Skill, len(s.skills))
	copy(out, s.skills)
	return out
}

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

// PrepareSkill turns /skill:name into a user message that already contains SKILL.md.
// The agent loop stays unaware of skills. The command just sends a normal prompt.
func (s *Session) PrepareSkill(name, request string) (string, error) {
	skill, ok := s.lookup(name)
	if !ok {
		return "", fmt.Errorf("unknown skill %q", name)
	}
	body, err := os.ReadFile(skill.Path)
	if err != nil {
		return "", err
	}
	return skills.Invocation(skill, string(body), request), nil
}

func (s *Session) lookup(name string) (skills.Skill, bool) {
	for _, skill := range s.skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return skills.Skill{}, false
}

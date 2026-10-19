package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/skills"
)

// Config selects the model endpoint and the workspace.
// The caller passes Model and BaseURL. Only an empty Workspace becomes ".".
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
	// Yolo skips file-change review and the dangerous-command check.
	// It is the one switch for "do not ask".
	Yolo bool
	// SessionDir is the sessions root, the parent of the per-workspace directory.
	// Empty leaves the transcript in memory.
	SessionDir string
	// Resume is empty for a new session, ResumeLatest, or a path, id, or unique prefix.
	Resume string
	// Compact is the context budget. A zero window uses the agent defaults.
	Compact agent.CompactSettings
}

// Confirm is one question shown before a tool runs.
// Title is the prompt. Body is the command or the diff. Detail is the short line under it.
type Confirm struct {
	Title  string
	Body   string
	Detail string
}

// Session is one coding-agent conversation in a workspace.
type Session struct {
	agent   *agent.Agent
	ws      *Workspace
	model   string
	skills  []skills.Skill
	yolo    bool
	journal *journal
	confirm func(ctx context.Context, prompt Confirm) (bool, error)
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
	// The hook closes over this session so SetConfirm can be called after New.
	session := &Session{
		ws:     ws,
		model:  cfg.Model,
		skills: found,
		yolo:   cfg.Yolo,
	}
	var prior []ai.Message
	var record func(ai.Message) error
	var afterCompact func(*agent.Compaction) error
	if strings.TrimSpace(cfg.SessionDir) != "" {
		j, msgs, err := openJournal(cfg.SessionDir, ws.Root(), cfg.Resume)
		if err != nil {
			return nil, err
		}
		session.journal = j
		prior = msgs
		record = func(msg ai.Message) error {
			if session.journal == nil {
				return nil
			}
			return session.journal.append(msg, session.model)
		}
		afterCompact = func(c *agent.Compaction) error {
			if session.journal == nil {
				return nil
			}
			return session.journal.appendCompaction(c)
		}
	}
	ag, err := agent.New(agent.Config{
		SystemPrompt: buildSystemPrompt(ws.Root(), found),
		Model:        cfg.Model,
		Tools:        Tools(ws, skillDirs(found)),
		Stream:       stream,
		MaxTurns:     cfg.MaxTurns,
		Before:       session.beforeTool,
		Record:       record,
		AfterCompact: afterCompact,
		Compact:      cfg.Compact,
	})
	if err != nil {
		session.closeJournal()
		return nil, err
	}
	if session.journal != nil && (len(prior) > 0 || session.journal.compact != nil) {
		ag.Restore(prior, session.journal.msgIDs, session.journal.compact)
	}
	session.agent = ag
	return session, nil
}

func (s *Session) closeJournal() {
	if s.journal != nil {
		s.journal.close()
	}
}

// SetConfirm installs the interactive check. Call it before Prompt.
// A nil confirm denies dangerous commands and file changes.
// The TUI is the only caller that asks. -p and the line session leave it nil.
func (s *Session) SetConfirm(fn func(ctx context.Context, prompt Confirm) (bool, error)) {
	s.confirm = fn
}

func (s *Session) beforeTool(ctx context.Context, call ai.ToolCall) (agent.Decision, error) {
	// Yolo skips every question. The command denylist and file review share it.
	if s.yolo {
		return agent.Decision{}, nil
	}
	switch call.Name {
	case runCommandTool:
		return s.guardCommand(ctx, call)
	case editToolName, writeToolName:
		return s.reviewFile(ctx, call)
	default:
		return agent.Decision{}, nil
	}
}

func (s *Session) guardCommand(ctx context.Context, call ai.ToolCall) (agent.Decision, error) {
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
	allow, err := s.confirm(ctx, Confirm{
		Title:  "! allow this command?",
		Body:   args.Command,
		Detail: reason,
	})
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
	if strings.TrimSpace(c.Workspace) == "" {
		c.Workspace = "."
	}
	return c
}

func (c Config) stream() (ai.StreamFunc, error) {
	if c.Stream != nil {
		return c.Stream, nil
	}
	client := &ai.Client{BaseURL: c.BaseURL, APIKey: c.APIKey}
	return client.Stream, nil
}

// Root is the workspace directory.
func (s *Session) Root() string { return s.ws.Root() }

// Model is the model name sent on each request.
func (s *Session) Model() string { return s.model }

// Messages is a copy of the transcript, including messages restored from a session file.
func (s *Session) Messages() []ai.Message { return s.agent.Messages() }

// SessionID is the id in the session header. It is empty when persistence is off.
func (s *Session) SessionID() string {
	if s.journal == nil {
		return ""
	}
	return s.journal.id
}

// SessionFile is the JSONL path. It is empty until the first stored message,
// or until an existing file is resumed.
func (s *Session) SessionFile() string {
	if s.journal == nil {
		return ""
	}
	return s.journal.path
}

// Usage returns the latest model call and the session sum.
func (s *Session) Usage() agent.Usage { return s.agent.Usage() }

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

// Reset clears the live transcript and the in-memory token total.
// When a session file is open, the next prompt starts a new file.
// The previous file stays on disk.
func (s *Session) Reset() error {
	s.agent.Reset()
	if s.journal == nil {
		return nil
	}
	dir, cwd := s.journal.dir, s.journal.cwd
	s.journal.close()
	next, err := newJournal(dir, cwd)
	if err != nil {
		s.journal = nil
		return err
	}
	s.journal = next
	return nil
}

// Compact summarizes older messages and keeps the recent window.
// The next Prompt sends that summary plus the tail. instructions focus
// the summary. An error leaves the context unchanged. When a session file
// is open, the summary is appended and becomes the leaf.
func (s *Session) Compact(ctx context.Context, instructions string) error {
	return s.agent.Compact(ctx, instructions)
}

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

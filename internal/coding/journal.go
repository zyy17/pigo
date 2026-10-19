package coding

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zyy17/pigo/internal/ai"
)

// ResumeLatest opens the newest session for the workspace.
// An empty Resume starts a new file. Any other value is a path, an id, or a unique id prefix.
const ResumeLatest = "continue"

// sessionVersion is Pi's current header version. pigo writes a single chain,
// which is a valid tree with one leaf. It does not write branches or compaction.
const sessionVersion = 3

// journal is one append-only session file.
// The file is created on the first user, assistant, or tool-result message,
// so opening pigo and quitting without a prompt leaves no empty file.
type journal struct {
	dir     string
	cwd     string
	id      string
	path    string
	leaf    string
	started time.Time
	file    *os.File
	ids     map[string]struct{}
	flushed bool
}

// DefaultSessionRoot is ~/.pigo/sessions.
// Pi stores sessions under ~/.pi/agent/sessions. pigo uses its own directory
// with the same per-workspace layout, so the two tools do not open each other's files.
func DefaultSessionRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", errors.New("home directory is empty")
	}
	return filepath.Join(home, ".pigo", "sessions"), nil
}

// sessionBucket is ~/.pigo/sessions/--<cwd>--/.
// The encoding matches Pi: drop one leading separator, then turn /, \, and : into -.
func sessionBucket(root, cwd string) string {
	return filepath.Join(root, encodeProject(cwd))
}

func encodeProject(cwd string) string {
	clean := filepath.Clean(cwd)
	s := strings.TrimPrefix(clean, string(filepath.Separator))
	s = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(s)
	return "--" + s + "--"
}

func openJournal(root, cwd, resume string) (*journal, []ai.Message, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	switch strings.TrimSpace(resume) {
	case "":
		j, err := newJournal(root, cwd)
		return j, nil, err
	case ResumeLatest:
		path, err := newestSession(sessionBucket(root, cwd))
		if err != nil {
			return nil, nil, err
		}
		if path == "" {
			j, err := newJournal(root, cwd)
			return j, nil, err
		}
		return loadJournal(root, cwd, path)
	default:
		path, err := resolveSession(root, cwd, resume)
		if err != nil {
			return nil, nil, err
		}
		return loadJournal(root, cwd, path)
	}
}

func newJournal(root, cwd string) (*journal, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	return &journal{
		dir:     root,
		cwd:     cwd,
		id:      id,
		started: time.Now().UTC(),
		ids:     map[string]struct{}{},
	}, nil
}

func (j *journal) close() {
	if j.file != nil {
		_ = j.file.Close()
		j.file = nil
	}
}

func (j *journal) append(msg ai.Message, model string) error {
	switch msg.Role {
	case ai.RoleUser, ai.RoleAssistant, ai.RoleToolResult:
	default:
		return nil
	}
	entry, err := j.messageEntry(msg, model)
	if err != nil {
		return err
	}
	if !j.flushed {
		return j.create(entry)
	}
	if err := j.writeLine(entry); err != nil {
		return err
	}
	j.leaf = entry.ID
	return nil
}

func (j *journal) messageEntry(msg ai.Message, model string) (sessionEntry, error) {
	id, err := j.allocID()
	if err != nil {
		return sessionEntry{}, err
	}
	body, err := marshalMessage(msg, model)
	if err != nil {
		return sessionEntry{}, err
	}
	var parent *string
	if j.leaf != "" {
		p := j.leaf
		parent = &p
	}
	return sessionEntry{
		Type:      "message",
		ID:        id,
		ParentID:  parent,
		Timestamp: isoTime(time.Now().UTC()),
		Message:   body,
	}, nil
}

func (j *journal) allocID() (string, error) {
	if j.ids == nil {
		j.ids = map[string]struct{}{}
	}
	for range 8 {
		id, err := newEntryID()
		if err != nil {
			return "", err
		}
		if _, ok := j.ids[id]; ok {
			continue
		}
		j.ids[id] = struct{}{}
		return id, nil
	}
	return "", errors.New("session entry id collision")
}

func (j *journal) create(entry sessionEntry) error {
	if err := os.MkdirAll(sessionBucket(j.dir, j.cwd), 0o700); err != nil {
		return err
	}
	path := filepath.Join(sessionBucket(j.dir, j.cwd), j.fileName())
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	j.file = f
	j.path = path
	if err := j.writeLine(j.header()); err != nil {
		j.abortCreate(path)
		return err
	}
	if err := j.writeLine(entry); err != nil {
		j.abortCreate(path)
		return err
	}
	j.leaf = entry.ID
	j.flushed = true
	return nil
}

func (j *journal) abortCreate(path string) {
	j.close()
	j.path = ""
	_ = os.Remove(path)
}

func (j *journal) header() sessionHeader {
	return sessionHeader{
		Type:      "session",
		Version:   sessionVersion,
		ID:        j.id,
		Timestamp: isoTime(j.started),
		Cwd:       j.cwd,
	}
}

func (j *journal) fileName() string {
	stamp := strings.NewReplacer(":", "-", ".", "-").Replace(isoTime(j.started))
	return stamp + "_" + j.id + ".jsonl"
}

func (j *journal) writeLine(v any) error {
	b, err := marshalJSON(v)
	if err != nil {
		return err
	}
	if _, err := j.file.Write(append(b, '\n')); err != nil {
		return err
	}
	return j.file.Sync()
}

func newestSession(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var best string
	var bestTime time.Time
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		if _, err := readHeader(path); err != nil {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		mt := info.ModTime()
		if best == "" || mt.After(bestTime) || (mt.Equal(bestTime) && path > best) {
			best = path
			bestTime = mt
		}
	}
	return best, nil
}

func resolveSession(root, cwd, spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", errors.New("session is empty")
	}
	if isSessionPath(spec) {
		path, err := filepath.Abs(spec)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("session file: %w", err)
		}
		return path, nil
	}
	return findSession(sessionBucket(root, cwd), spec)
}

func isSessionPath(spec string) bool {
	return strings.ContainsAny(spec, `/\`) || strings.HasSuffix(strings.ToLower(spec), ".jsonl")
}

func findSession(dir, spec string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("session %q was not found", spec)
	}
	if err != nil {
		return "", err
	}
	var exact string
	var prefixes []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		header, err := readHeader(path)
		if err != nil {
			continue
		}
		if header.ID == spec {
			exact = path
			break
		}
		if strings.HasPrefix(header.ID, spec) {
			prefixes = append(prefixes, path)
		}
	}
	if exact != "" {
		return exact, nil
	}
	switch len(prefixes) {
	case 1:
		return prefixes[0], nil
	case 0:
		return "", fmt.Errorf("session %q was not found", spec)
	default:
		return "", fmt.Errorf("session %q matches %d sessions", spec, len(prefixes))
	}
}

func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func newEntryID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func isoTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// sessionHeader is the first line. It is metadata, not a tree node.
type sessionHeader struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}

// sessionEntry is one tree node. parentId is null for the root message.
type sessionEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	ParentID  *string         `json:"parentId"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message,omitempty"`
}

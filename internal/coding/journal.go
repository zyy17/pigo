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

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

// ResumeLatest opens the newest session for the workspace.
// An empty Resume starts a new file. Any other value is a path, an id, or a unique id prefix.
const ResumeLatest = "continue"

// sessionVersion is Pi's current header version. pigo writes a single chain,
// which is a valid tree with one leaf. It does not write branches. A
// compaction summary is appended as its own entry and becomes the leaf.
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
	// msgIDs[i] is the file id of agent message i. An empty string means
	// that message never landed, so later indexes still match the transcript.
	// A compaction entry stores msgIDs[From], not the agent's in-memory mN name.
	msgIDs  []string
	compact *agent.Compaction
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
		j.noteMissing()
		return nil
	}
	entry, err := j.messageEntry(msg, model)
	if err != nil {
		j.noteMissing()
		return err
	}
	if !j.flushed {
		if err := j.create(entry); err != nil {
			j.noteMissing()
			return err
		}
		j.msgIDs = append(j.msgIDs, entry.ID)
		return nil
	}
	if err := j.writeLine(entry); err != nil {
		j.noteMissing()
		return err
	}
	j.leaf = entry.ID
	j.msgIDs = append(j.msgIDs, entry.ID)
	return nil
}

// noteMissing keeps msgIDs aligned with the in-memory transcript when a
// message is not in the file. Compaction indexes this slice by message number.
func (j *journal) noteMissing() {
	j.msgIDs = append(j.msgIDs, "")
}

// appendCompaction writes the summary as the new leaf. The messages it
// replaces stay in the file. firstKeptEntryId is the file id of the first
// message the next request still sends. A later message parents this entry,
// so a resume walks through the summary to the kept tail.
func (j *journal) appendCompaction(c *agent.Compaction) error {
	if c == nil || !j.flushed {
		return nil
	}
	if c.From < 0 || c.From > len(j.msgIDs) {
		return fmt.Errorf("compaction kept message %d is not in the session file", c.From)
	}
	kept := ""
	if c.From < len(j.msgIDs) {
		kept = j.msgIDs[c.From]
	}
	// From == len keeps nothing. An empty slot means that message never
	// landed, so writing the summary would point the leaf at the wrong line.
	if kept == "" && c.From != len(j.msgIDs) {
		return fmt.Errorf("compaction kept message %d is not in the session file", c.From)
	}
	id, err := j.allocID()
	if err != nil {
		return err
	}
	var parent *string
	if j.leaf != "" {
		p := j.leaf
		parent = &p
	}
	entry := sessionEntry{
		Type:             "compaction",
		ID:               id,
		ParentID:         parent,
		Timestamp:        isoTime(time.Now().UTC()),
		Summary:          c.Summary,
		FirstKeptEntryID: kept,
		TokensBefore:     c.TokensBefore,
		Details:          compactionDetailsOf(c),
	}
	if err := j.writeLine(entry); err != nil {
		return err
	}
	j.leaf = entry.ID
	return nil
}

func compactionDetailsOf(c *agent.Compaction) *compactionDetails {
	if c == nil || (len(c.ReadFiles) == 0 && len(c.ModifiedFiles) == 0) {
		return nil
	}
	return &compactionDetails{
		ReadFiles:     append([]string(nil), c.ReadFiles...),
		ModifiedFiles: append([]string(nil), c.ModifiedFiles...),
	}
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
	// Append, and read-write: a later line may have to read and cut a torn
	// tail. O_APPEND keeps that write at the new end after the cut.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|os.O_APPEND, 0o600)
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
	prefix, err := j.repairTail()
	if err != nil {
		return err
	}
	line := make([]byte, 0, len(prefix)+len(b)+1)
	line = append(line, prefix...)
	line = append(line, b...)
	line = append(line, '\n')
	if _, err := j.file.Write(line); err != nil {
		return err
	}
	return j.file.Sync()
}

// repairTail prepares the file so the next write starts a line of its own.
// An unfinished fragment is cut back to the previous newline. A finished
// JSON value that is only missing its newline stays, and the prefix is that
// newline. A file that already ends on a newline needs nothing.
func (j *journal) repairTail() ([]byte, error) {
	info, err := j.file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}
	last := make([]byte, 1)
	if _, err := j.file.ReadAt(last, size-1); err != nil {
		return nil, err
	}
	if last[0] == '\n' {
		return nil, nil
	}
	data := make([]byte, size)
	if _, err := j.file.ReadAt(data, 0); err != nil {
		return nil, err
	}
	nl := bytes.LastIndexByte(data, '\n')
	var frag []byte
	if nl >= 0 {
		frag = data[nl+1:]
	} else {
		frag = data
	}
	if json.Valid(bytes.TrimSpace(frag)) {
		return []byte{'\n'}, nil
	}
	if nl < 0 {
		return nil, errors.New("session file has no complete line")
	}
	if err := j.file.Truncate(int64(nl + 1)); err != nil {
		return nil, err
	}
	if err := j.file.Sync(); err != nil {
		return nil, err
	}
	return nil, nil
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
		// Header only. Continue loads this file next, so a bad line in it
		// is reported there.
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
// Compaction fields are omitted on a message line.
type sessionEntry struct {
	Type             string             `json:"type"`
	ID               string             `json:"id"`
	ParentID         *string            `json:"parentId"`
	Timestamp        string             `json:"timestamp"`
	Message          json.RawMessage    `json:"message,omitempty"`
	Summary          string             `json:"summary,omitempty"`
	FirstKeptEntryID string             `json:"firstKeptEntryId,omitempty"`
	TokensBefore     int                `json:"tokensBefore,omitempty"`
	Details          *compactionDetails `json:"details,omitempty"`
}

// compactionDetails lists files the summary replaced. The names match Pi.
type compactionDetails struct {
	ReadFiles     []string `json:"readFiles,omitempty"`
	ModifiedFiles []string `json:"modifiedFiles,omitempty"`
}

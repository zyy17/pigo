package coding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

// fileLine is one JSONL object. Header lines and tree entries share it
// so the loader can branch on type before copying into a narrower struct.
type fileLine struct {
	Type             string             `json:"type"`
	Version          int                `json:"version"`
	ID               string             `json:"id"`
	ParentID         *string            `json:"parentId"`
	Timestamp        string             `json:"timestamp"`
	Cwd              string             `json:"cwd"`
	Message          json.RawMessage    `json:"message"`
	Summary          string             `json:"summary"`
	FirstKeptEntryID string             `json:"firstKeptEntryId"`
	TokensBefore     int                `json:"tokensBefore"`
	Details          *compactionDetails `json:"details"`
}

func loadJournal(root, cwd, path string) (*journal, []ai.Message, error) {
	header, entries, err := readEntries(path)
	if err != nil {
		return nil, nil, err
	}
	msgs, ids, compact, leaf, err := transcriptFrom(entries)
	if err != nil {
		return nil, nil, fmt.Errorf("session %s: %w", path, err)
	}
	started, err := time.Parse(time.RFC3339Nano, header.Timestamp)
	if err != nil {
		started = time.Now().UTC()
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	j := &journal{
		dir:     root,
		cwd:     cwd,
		id:      header.ID,
		path:    path,
		leaf:    leaf,
		started: started.UTC(),
		file:    f,
		ids:     map[string]struct{}{},
		flushed: true,
		msgIDs:  ids,
		compact: compact,
	}
	for _, entry := range entries {
		if entry.ID != "" {
			j.ids[entry.ID] = struct{}{}
		}
	}
	return j, msgs, nil
}

func readHeader(path string) (sessionHeader, error) {
	header, _, err := readEntries(path)
	return header, err
}

func readEntries(path string) (sessionHeader, []sessionEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionHeader{}, nil, err
	}
	lines := jsonlLines(data)
	if len(lines) == 0 {
		return sessionHeader{}, nil, fmt.Errorf("session %s is empty", path)
	}
	// A crash can tear the last line. Drop that fragment and keep the
	// complete lines. A bad line in the middle is a corrupt file.
	if !json.Valid(lines[len(lines)-1].text) && !bytes.HasSuffix(data, []byte("\n")) {
		lines = lines[:len(lines)-1]
		if len(lines) == 0 {
			return sessionHeader{}, nil, fmt.Errorf("session %s is empty", path)
		}
	}
	lines[0].text = bytes.TrimPrefix(lines[0].text, []byte("\uFEFF"))

	var headerLine fileLine
	if err := json.Unmarshal(lines[0].text, &headerLine); err != nil {
		return sessionHeader{}, nil, fmt.Errorf("session %s:%d: %w", path, lines[0].n, err)
	}
	if headerLine.Type != "session" || headerLine.ID == "" {
		return sessionHeader{}, nil, fmt.Errorf("session %s is not a pigo session", path)
	}
	header := sessionHeader{
		Type:      headerLine.Type,
		Version:   headerLine.Version,
		ID:        headerLine.ID,
		Timestamp: headerLine.Timestamp,
		Cwd:       headerLine.Cwd,
	}
	if header.Version == 0 {
		header.Version = 1
	}

	entries := make([]sessionEntry, 0, len(lines)-1)
	for _, line := range lines[1:] {
		var raw fileLine
		if err := json.Unmarshal(line.text, &raw); err != nil {
			return sessionHeader{}, nil, fmt.Errorf("session %s:%d: %w", path, line.n, err)
		}
		if raw.Type == "session" {
			return sessionHeader{}, nil, fmt.Errorf("session %s:%d: extra session header", path, line.n)
		}
		entries = append(entries, sessionEntry{
			Type:             raw.Type,
			ID:               raw.ID,
			ParentID:         raw.ParentID,
			Timestamp:        raw.Timestamp,
			Message:          raw.Message,
			Summary:          raw.Summary,
			FirstKeptEntryID: raw.FirstKeptEntryID,
			TokensBefore:     raw.TokensBefore,
			Details:          raw.Details,
		})
	}
	return header, entries, nil
}

type jsonlLine struct {
	n    int
	text []byte
}

func jsonlLines(data []byte) []jsonlLine {
	raw := bytes.Split(data, []byte("\n"))
	if len(raw) > 0 && len(bytes.TrimSpace(raw[len(raw)-1])) == 0 {
		raw = raw[:len(raw)-1]
	}
	lines := make([]jsonlLine, 0, len(raw))
	for i, line := range raw {
		text := bytes.TrimSpace(line)
		if len(text) == 0 {
			continue
		}
		lines = append(lines, jsonlLine{n: i + 1, text: text})
	}
	return lines
}

// transcriptFrom walks the active leaf back to its root.
// The leaf is the last line, which is where pigo appends. A side branch
// stays in the file and is not sent to the model. Entries other than
// message (usage, label, compaction) stay in the walk so the chain
// connects, and they do not become transcript messages.
//
// The latest compaction entry on that path is returned with the messages.
// firstKeptEntryId selects the first message the next request sends. The
// messages themselves stay in the slice. A compaction that is not on the
// leaf path is ignored. If that id is missing, the kept tail is whatever
// survived after the compaction entry.
//
// A file with no parentId links is Pi's version 1 shape: file order is the path.
func transcriptFrom(entries []sessionEntry) ([]ai.Message, []string, *agent.Compaction, string, error) {
	if len(entries) == 0 {
		return nil, nil, nil, "", nil
	}
	byID := make(map[string]sessionEntry, len(entries))
	linked := false
	for _, entry := range entries {
		if entry.ID == "" {
			return nil, nil, nil, "", fmt.Errorf("entry is missing an id")
		}
		if _, ok := byID[entry.ID]; ok {
			return nil, nil, nil, "", fmt.Errorf("duplicate entry %s", entry.ID)
		}
		byID[entry.ID] = entry
		if entry.ParentID != nil && *entry.ParentID != "" {
			linked = true
		}
	}
	leaf := entries[len(entries)-1].ID
	ordered := entries
	if linked {
		var err error
		ordered, err = walkLeaf(byID, leaf)
		if err != nil {
			return nil, nil, nil, "", err
		}
	}
	msgs := make([]ai.Message, 0, len(ordered))
	ids := make([]string, 0, len(ordered))
	for _, entry := range ordered {
		if entry.Type != "message" {
			continue
		}
		msg, skip, err := decodeMessage(entry.Message)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("entry %s: %w", entry.ID, err)
		}
		if skip {
			continue
		}
		msgs = append(msgs, msg)
		ids = append(ids, entry.ID)
	}
	msgs, ids = modelTranscript(msgs, ids)
	return msgs, ids, projectCompaction(ordered, ids), leaf, nil
}

// projectCompaction turns the latest summary on the leaf into the cut the
// agent applies. The index is into the messages that survived load, which
// can drop a torn tool pair the file still holds.
func projectCompaction(ordered []sessionEntry, ids []string) *agent.Compaction {
	var entry *sessionEntry
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].Type != "compaction" || strings.TrimSpace(ordered[i].Summary) == "" {
			continue
		}
		found := ordered[i]
		entry = &found
		break
	}
	if entry == nil {
		return nil
	}
	indexOf := make(map[string]int, len(ids))
	for i, id := range ids {
		if id != "" {
			indexOf[id] = i
		}
	}
	from, kept := keptIndex(ordered, entry, indexOf, len(ids))
	c := &agent.Compaction{
		ID:           entry.ID,
		Summary:      strings.TrimSpace(entry.Summary),
		FirstKeptID:  kept,
		From:         from,
		TokensBefore: entry.TokensBefore,
	}
	if entry.Details != nil {
		c.ReadFiles = append([]string(nil), entry.Details.ReadFiles...)
		c.ModifiedFiles = append([]string(nil), entry.Details.ModifiedFiles...)
	}
	return c
}

func keptIndex(ordered []sessionEntry, compact *sessionEntry, indexOf map[string]int, n int) (int, string) {
	if id := compact.FirstKeptEntryID; id != "" {
		if i, ok := indexOf[id]; ok {
			return i, id
		}
		if i, id, ok := nextSurviving(ordered, id, indexOf); ok {
			return i, id
		}
	}
	if i, id, ok := nextSurviving(ordered, compact.ID, indexOf); ok {
		return i, id
	}
	return n, ""
}

func nextSurviving(ordered []sessionEntry, afterID string, indexOf map[string]int) (int, string, bool) {
	seen := false
	for _, entry := range ordered {
		if !seen {
			if entry.ID == afterID {
				seen = true
			}
			continue
		}
		if i, ok := indexOf[entry.ID]; ok {
			return i, entry.ID, true
		}
	}
	return 0, "", false
}

func walkLeaf(byID map[string]sessionEntry, leaf string) ([]sessionEntry, error) {
	chain := make([]sessionEntry, 0, len(byID))
	seen := map[string]bool{}
	for id := leaf; id != ""; {
		if seen[id] {
			return nil, fmt.Errorf("entry %s is part of a cycle", id)
		}
		seen[id] = true
		entry, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("missing entry %s", id)
		}
		chain = append(chain, entry)
		if entry.ParentID == nil || *entry.ParentID == "" {
			break
		}
		parent := *entry.ParentID
		if _, ok := byID[parent]; !ok {
			return nil, fmt.Errorf("entry %s points at missing parent %s", entry.ID, parent)
		}
		id = parent
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// modelTranscript drops tool calls that have no stored result, and tool
// results that have no call. A crash can leave the pair unfinished. Sending
// that pair would make the next request invalid. The file is not rewritten.
func modelTranscript(msgs []ai.Message, ids []string) ([]ai.Message, []string) {
	out := make([]ai.Message, 0, len(msgs))
	outIDs := make([]string, 0, len(msgs))
	seenResult := map[string]bool{}
	for i, msg := range msgs {
		switch msg.Role {
		case ai.RoleUser:
			if strings.TrimSpace(msg.Text) == "" {
				continue
			}
			out = append(out, msg)
			outIDs = append(outIDs, ids[i])
		case ai.RoleAssistant:
			msg.ToolCalls = keptCalls(msgs, i, msg.ToolCalls)
			if len(msg.ToolCalls) == 0 && strings.TrimSpace(msg.Text) == "" {
				continue
			}
			if len(msg.ToolCalls) == 0 && msg.StopReason == ai.StopToolUse {
				msg.StopReason = ai.StopStop
			}
			out = append(out, msg)
			outIDs = append(outIDs, ids[i])
		case ai.RoleToolResult:
			if msg.ToolCallID == "" || seenResult[msg.ToolCallID] {
				continue
			}
			if !callBefore(msgs, i, msg.ToolCallID) {
				continue
			}
			seenResult[msg.ToolCallID] = true
			out = append(out, msg)
			outIDs = append(outIDs, ids[i])
		}
	}
	return out, outIDs
}

func keptCalls(msgs []ai.Message, index int, calls []ai.ToolCall) []ai.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	kept := make([]ai.ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.ID == "" || !resultAfter(msgs, index, call.ID) {
			continue
		}
		kept = append(kept, call)
	}
	return kept
}

func resultAfter(msgs []ai.Message, index int, id string) bool {
	for _, msg := range msgs[index+1:] {
		if msg.Role == ai.RoleToolResult && msg.ToolCallID == id {
			return true
		}
	}
	return false
}

func callBefore(msgs []ai.Message, index int, id string) bool {
	for _, msg := range msgs[:index] {
		if msg.Role != ai.RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			if call.ID == id {
				return true
			}
		}
	}
	return false
}

type diskMessage struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	ToolCallID   string          `json:"toolCallId"`
	ToolName     string          `json:"toolName"`
	IsError      bool            `json:"isError"`
	StopReason   ai.StopReason   `json:"stopReason"`
	ErrorMessage string          `json:"errorMessage"`
}

type diskBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type userDisk struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type assistantDisk struct {
	Role         string      `json:"role"`
	Content      []diskBlock `json:"content"`
	Model        string      `json:"model,omitempty"`
	StopReason   string      `json:"stopReason,omitempty"`
	ErrorMessage string      `json:"errorMessage,omitempty"`
	Timestamp    int64       `json:"timestamp"`
}

type toolDisk struct {
	Role       string      `json:"role"`
	ToolCallID string      `json:"toolCallId"`
	ToolName   string      `json:"toolName"`
	Content    []diskBlock `json:"content"`
	IsError    bool        `json:"isError"`
	Timestamp  int64       `json:"timestamp"`
}

func marshalMessage(msg ai.Message, model string) ([]byte, error) {
	now := time.Now().UnixMilli()
	switch msg.Role {
	case ai.RoleUser:
		return marshalJSON(userDisk{Role: string(msg.Role), Content: msg.Text, Timestamp: now})
	case ai.RoleAssistant:
		return marshalJSON(assistantDisk{
			Role:         string(msg.Role),
			Content:      assistantBlocks(msg),
			Model:        model,
			StopReason:   string(msg.StopReason),
			ErrorMessage: msg.Error,
			Timestamp:    now,
		})
	case ai.RoleToolResult:
		return marshalJSON(toolDisk{
			Role:       string(msg.Role),
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			Content:    []diskBlock{{Type: "text", Text: msg.Text}},
			IsError:    msg.IsError,
			Timestamp:  now,
		})
	default:
		return nil, fmt.Errorf("cannot store role %q", msg.Role)
	}
}

func assistantBlocks(msg ai.Message) []diskBlock {
	var blocks []diskBlock
	if msg.Text != "" {
		blocks = append(blocks, diskBlock{Type: "text", Text: msg.Text})
	}
	for _, call := range msg.ToolCalls {
		blocks = append(blocks, diskBlock{
			Type:      "toolCall",
			ID:        call.ID,
			Name:      call.Name,
			Arguments: argRaw(call.Arguments),
		})
	}
	if blocks == nil {
		blocks = []diskBlock{}
	}
	return blocks
}

func argRaw(b []byte) json.RawMessage {
	b = bytes.TrimSpace(b)
	if json.Valid(b) && len(b) > 0 {
		return json.RawMessage(append([]byte(nil), b...))
	}
	return json.RawMessage(`{}`)
}

// decodeMessage turns one stored AgentMessage into the flat transcript.
// System messages are skipped: the live prompt is attached when the request
// is built. Unknown roles and thinking or image blocks are skipped too.
func decodeMessage(raw json.RawMessage) (ai.Message, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ai.Message{}, true, nil
	}
	var meta diskMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ai.Message{}, false, err
	}
	switch ai.Role(meta.Role) {
	case ai.RoleSystem:
		return ai.Message{}, true, nil
	case ai.RoleUser:
		text, err := contentText(meta.Content)
		if err != nil {
			return ai.Message{}, false, err
		}
		return ai.User(text), false, nil
	case ai.RoleAssistant:
		text, calls, err := contentAssistant(meta.Content)
		if err != nil {
			return ai.Message{}, false, err
		}
		msg := ai.Assistant(text, meta.StopReason, calls...)
		msg.Error = meta.ErrorMessage
		return msg, false, nil
	case ai.RoleToolResult:
		text, err := contentText(meta.Content)
		if err != nil {
			return ai.Message{}, false, err
		}
		return ai.ToolResult(meta.ToolCallID, meta.ToolName, text, meta.IsError), false, nil
	default:
		return ai.Message{}, true, nil
	}
}

func contentText(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	if raw[0] == '[' {
		blocks, err := decodeBlocks(raw)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, block := range blocks {
			if block.Type == "text" {
				b.WriteString(block.Text)
			}
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("unsupported content")
}

func contentAssistant(raw json.RawMessage) (string, []ai.ToolCall, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil, nil
	}
	if raw[0] == '"' {
		text, err := contentText(raw)
		return text, nil, err
	}
	blocks, err := decodeBlocks(raw)
	if err != nil {
		return "", nil, err
	}
	var text strings.Builder
	var calls []ai.ToolCall
	for _, block := range blocks {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "toolCall":
			calls = append(calls, ai.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: argBytes(block.Arguments),
			})
		}
	}
	return text.String(), calls, nil
}

func decodeBlocks(raw json.RawMessage) ([]diskBlock, error) {
	var blocks []diskBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

func argBytes(raw json.RawMessage) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return []byte("{}")
		}
		if json.Valid([]byte(text)) {
			return []byte(text)
		}
		return []byte("{}")
	}
	return append([]byte(nil), raw...)
}

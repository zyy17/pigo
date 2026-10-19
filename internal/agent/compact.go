package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/zyy17/pigo/internal/ai"
)

// Defaults match Pi's reserve and recent-window sizes. Pi reads the context
// window from the model; pigo has no model catalog, so the window is a setting.
const (
	DefaultContextWindow    = 128_000
	DefaultReserveTokens    = 16_384
	DefaultKeepRecentTokens = 20_000
)

// ErrNothingToCompact means a manual compact found nothing older than the
// recent window.
var ErrNothingToCompact = errors.New("nothing to compact")

// CompactSettings decides when to summarize and how much of the tail to keep.
// A zero ContextWindow selects the defaults, including auto-compact.
// ReserveTokens and KeepRecentTokens may be zero when the window is set:
// zero reserve compacts as soon as the estimate passes the window, and zero
// keep still leaves the newest legal cut in place so a tool pair is not split.
type CompactSettings struct {
	Enabled          bool
	ContextWindow    int
	ReserveTokens    int
	KeepRecentTokens int
}

// DefaultCompactSettings is the built-in budget.
func DefaultCompactSettings() CompactSettings {
	return CompactSettings{
		Enabled:          true,
		ContextWindow:    DefaultContextWindow,
		ReserveTokens:    DefaultReserveTokens,
		KeepRecentTokens: DefaultKeepRecentTokens,
	}
}

func (s CompactSettings) orDefault() CompactSettings {
	if s.ContextWindow <= 0 {
		return DefaultCompactSettings()
	}
	if s.ReserveTokens < 0 {
		s.ReserveTokens = DefaultReserveTokens
	}
	if s.KeepRecentTokens < 0 {
		s.KeepRecentTokens = DefaultKeepRecentTokens
	}
	return s
}

// Compaction is the latest summary entry. Older messages stay in the
// transcript. Summary replaces everything before FirstKeptID on the next
// request. TokensBefore is the estimate of the context that was replaced,
// and the file lists are the details. From is the index of the first kept
// message. FirstKeptID is that message's in-memory name. The session file
// records its own entry id for the same index.
type Compaction struct {
	ID            string
	Summary       string
	FirstKeptID   string
	From          int
	TokensBefore  int
	ReadFiles     []string
	ModifiedFiles []string
}

func copyCompaction(c *Compaction) *Compaction {
	if c == nil {
		return nil
	}
	out := *c
	out.ReadFiles = append([]string(nil), c.ReadFiles...)
	out.ModifiedFiles = append([]string(nil), c.ModifiedFiles...)
	return &out
}

// contextAnchor is the last chat call's provider usage. It counts the prompt
// that call saw. Messages appended after it are estimated. A summary call
// does not move the anchor: its usage is the summarization prompt, not the
// chat context. Compaction clears it, because that count included messages
// the next request will not send.
type contextAnchor struct {
	tokens int
	after  int
	ok     bool
}

const (
	summaryPrefix = "The conversation history before this point was compacted into the following summary:\n\n<summary>\n"
	summarySuffix = "\n</summary>"

	// Tool results dominate the transcript. The summary prompt keeps a
	// prefix so the summarization request stays smaller than the context
	// it replaces.
	toolResultRunes = 2000
)

const summarySystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI assistant, then produce a structured summary following the exact format specified.

Do NOT continue the conversation. Do NOT respond to any questions in the conversation. ONLY output the structured summary.`

const summaryPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish?]

## Constraints & Preferences
- [Constraints, preferences, or requirements the user mentioned]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [What should happen next]

## Critical Context
- [Data needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

const updateSummaryPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary.

Update the existing structured summary. RULES:
- PRESERVE existing information from the previous summary
- ADD new progress, decisions, and context
- Move finished items from "In Progress" to "Done"
- UPDATE "Next Steps" from what was accomplished
- PRESERVE exact file paths, function names, and error messages

Use this EXACT format:

## Goal
[Existing goals, plus new ones if the task expanded]

## Constraints & Preferences
- [Existing constraints, plus new ones]

## Progress
### Done
- [x] [Previously done items and newly completed items]

### In Progress
- [ ] [Current work]

### Blocked
- [Current blockers]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [What should happen next]

## Critical Context
- [Context needed to continue]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

// Compact summarizes messages older than the recent window.
// instructions focus the summary. The call uses the same stream as a turn
// and does not append the summary as a transcript message. An error leaves
// the context unchanged. Compact is not safe to call during Prompt.
func (a *Agent) Compact(ctx context.Context, instructions string) error {
	ctx, cancel := context.WithCancel(ctx)
	a.setCancel(cancel)
	err := a.compact(ctx, strings.TrimSpace(instructions), true)
	if err != nil {
		// Failure emits nothing, so no listener has started another Prompt.
		a.clearCancel()
		cancel()
		if errors.Is(err, context.Canceled) {
			return ErrInterrupted
		}
		return err
	}
	cancel()
	return nil
}

// maybeCompact runs at the start of a model pass. The previous pass has
// already appended its tool results, which is the context the next call
// would send.
func (a *Agent) maybeCompact(ctx context.Context) error {
	return a.compact(ctx, "", false)
}

func (a *Agent) compact(ctx context.Context, instructions string, manual bool) error {
	err := a.doCompact(ctx, instructions, manual)
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrNothingToCompact) {
		return err
	}
	return fmt.Errorf("compaction: %w", err)
}

func (a *Agent) doCompact(ctx context.Context, instructions string, manual bool) error {
	if !manual && !a.settings.Enabled {
		return nil
	}
	plan, ok := a.prepare(manual)
	if !ok {
		if manual {
			return ErrNothingToCompact
		}
		return nil
	}
	summary, usage, err := a.summarize(ctx, plan, instructions)
	if err != nil {
		return err
	}
	read, modified := collectFiles(plan.summarize, plan.prevRead, plan.prevMod)
	summary = appendFileLists(summary, read, modified)
	prevCompact := a.state.compaction
	prevAnchor := a.anchor
	a.state.compactSeq++
	a.state.compaction = &Compaction{
		ID:            fmt.Sprintf("c%d", a.state.compactSeq),
		Summary:       summary,
		FirstKeptID:   plan.keptID,
		From:          plan.from,
		TokensBefore:  plan.tokens,
		ReadFiles:     read,
		ModifiedFiles: modified,
	}
	// The provider count included the messages this summary just replaced.
	a.anchor = contextAnchor{}

	var last, session *ai.Usage
	if usage != nil {
		last, session = a.noteCallUsage(usage)
	} else {
		_, session = a.usage.snapshot()
	}
	// Persist before listeners run. A failed write puts the summary back so
	// the next request still matches the file. The spent tokens stay counted.
	if a.afterCompact != nil {
		if err := a.afterCompact(copyCompaction(a.state.compaction)); err != nil {
			a.state.compaction = prevCompact
			a.anchor = prevAnchor
			a.state.compactSeq--
			return err
		}
	}
	// Manual compact drops its cancel before listeners run. A listener may
	// start Prompt from this event, and clearing afterwards would drop that
	// cancel. Auto-compact is already inside Prompt and must keep it.
	if manual {
		a.clearCancel()
	}
	a.emit(Event{Type: Compacted, Usage: last, Session: session})
	return nil
}

type compactPlan struct {
	summarize []ai.Message
	keptID    string
	from      int
	previous  string
	prevRead  []string
	prevMod   []string
	tokens    int
}

func (a *Agent) prepare(manual bool) (compactPlan, bool) {
	tokens := a.contextTokens()
	if !manual && !shouldCompact(tokens, a.settings.ContextWindow, a.settings.ReserveTokens) {
		return compactPlan{}, false
	}
	start := a.state.liveStart()
	span := a.state.Messages[start:]
	cut, ok := findCut(span, a.settings.KeepRecentTokens)
	if !ok {
		return compactPlan{}, false
	}
	plan := compactPlan{
		summarize: span[:cut],
		from:      start + cut,
		tokens:    tokens,
	}
	if plan.from < len(a.state.ids) {
		plan.keptID = a.state.ids[plan.from]
	}
	if c := a.state.compaction; c != nil {
		plan.previous = c.Summary
		plan.prevRead = c.ReadFiles
		plan.prevMod = c.ModifiedFiles
	}
	return plan, true
}

// shouldCompact reports that the estimate passed the free reserve below the window.
// An estimate equal to window-reserve does not compact.
func shouldCompact(tokens, window, reserve int) bool {
	return tokens > window-reserve
}

// contextTokens prefers the last chat call's provider usage, plus a character
// estimate of messages added since. Without that usage it estimates the
// projected messages. The estimate is about four characters per token, not
// the provider's tokenizer. The reserve absorbs the difference, and tool
// schemas are left in that reserve.
func (a *Agent) contextTokens() int {
	if a.anchor.ok {
		n := a.anchor.tokens
		if a.anchor.after < len(a.state.Messages) {
			n += estimateMessages(a.state.Messages[a.anchor.after:])
		}
		return n
	}
	return estimateText(a.state.SystemPrompt) + estimateMessages(a.context())
}

func (a *Agent) noteAnchor(call *ai.Usage) {
	if call == nil {
		return
	}
	n := call.Total
	if n == 0 {
		n = call.Input + call.Output
	}
	a.anchor = contextAnchor{tokens: n, after: len(a.state.Messages), ok: true}
}

func estimateMessages(msgs []ai.Message) int {
	n := 0
	for _, msg := range msgs {
		n += estimateMessage(msg)
	}
	return n
}

func estimateMessage(msg ai.Message) int {
	n := estimateText(msg.Text)
	for _, call := range msg.ToolCalls {
		n += estimateText(call.Name)
		n += estimateText(string(call.Arguments))
	}
	return n
}

func estimateText(s string) int {
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// findCut returns the index of the first message to keep verbatim.
// A tool result is not a cut: it stays with the assistant call that produced
// it. When the budget is crossed on a result, the cut moves forward to the
// next user or assistant message when there is one, and otherwise back to
// the assistant that owns the result.
func findCut(msgs []ai.Message, keepRecent int) (int, bool) {
	cuts := validCuts(msgs)
	if len(cuts) == 0 {
		return 0, false
	}
	acc := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		acc += estimateMessage(msgs[i])
		if acc < keepRecent {
			continue
		}
		if cut := firstAtOrAfter(cuts, i); cut > 0 {
			return cut, true
		}
		if cut := lastBefore(cuts, i); cut > 0 {
			return cut, true
		}
		return 0, false
	}
	return 0, false
}

func validCuts(msgs []ai.Message) []int {
	cuts := make([]int, 0, len(msgs))
	for i, msg := range msgs {
		if msg.Role == ai.RoleUser || msg.Role == ai.RoleAssistant {
			cuts = append(cuts, i)
		}
	}
	return cuts
}

func firstAtOrAfter(cuts []int, i int) int {
	for _, cut := range cuts {
		if cut >= i {
			return cut
		}
	}
	return -1
}

func lastBefore(cuts []int, i int) int {
	cut := -1
	for _, c := range cuts {
		if c >= i {
			break
		}
		cut = c
	}
	return cut
}

func (a *Agent) summarize(ctx context.Context, plan compactPlan, instructions string) (string, *ai.Usage, error) {
	req := ai.Request{
		Model: a.state.Model,
		Messages: []ai.Message{
			ai.System(summarySystemPrompt),
			ai.User(summaryUserPrompt(serializeConversation(plan.summarize), stripFileTags(plan.previous), instructions)),
		},
	}
	events, err := a.stream(ctx, req)
	if err != nil {
		return "", nil, err
	}
	var (
		done  ai.Message
		usage *ai.Usage
		saw   bool
	)
	// Text deltas are the summary being written. They are not chat output.
	for ev := range events {
		switch ev.Type {
		case ai.EventDone:
			done = ev.Message
			usage = ev.Usage
			saw = true
		case ai.EventError:
			if ev.Err != nil {
				return "", nil, ev.Err
			}
			return "", nil, errors.New("summary stream failed")
		}
	}
	if !saw {
		return "", nil, errors.New("summary stream ended early")
	}
	text := strings.TrimSpace(done.Text)
	if text == "" {
		if done.Error != "" {
			return "", nil, errors.New(done.Error)
		}
		return "", nil, errors.New("summary was empty")
	}
	return text, usage, nil
}

func summaryUserPrompt(conversation, previous, instructions string) string {
	var b strings.Builder
	b.WriteString("<conversation>\n")
	b.WriteString(conversation)
	if conversation != "" && !strings.HasSuffix(conversation, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("</conversation>\n\n")
	if previous != "" {
		b.WriteString("<previous-summary>\n")
		b.WriteString(previous)
		if !strings.HasSuffix(previous, "\n") {
			b.WriteByte('\n')
		}
		b.WriteString("</previous-summary>\n\n")
		b.WriteString(updateSummaryPrompt)
	} else {
		b.WriteString(summaryPrompt)
	}
	if instructions != "" {
		b.WriteString("\n\nAdditional focus: ")
		b.WriteString(instructions)
	}
	return b.String()
}

func summaryMessage(summary string) ai.Message {
	return ai.User(summaryPrefix + summary + summarySuffix)
}

// serializeConversation turns the span into text so the model summarizes it
// instead of continuing it.
func serializeConversation(msgs []ai.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		writeSerialized(&b, msg)
	}
	return b.String()
}

func writeSerialized(b *strings.Builder, msg ai.Message) {
	switch msg.Role {
	case ai.RoleUser:
		writeLine(b, "[User]: "+msg.Text)
	case ai.RoleAssistant:
		if msg.Text != "" {
			writeLine(b, "[Assistant]: "+msg.Text)
		}
		if len(msg.ToolCalls) > 0 {
			writeLine(b, "[Assistant tool calls]: "+formatCalls(msg.ToolCalls))
		}
		if msg.Text == "" && len(msg.ToolCalls) == 0 {
			writeLine(b, "[Assistant]:")
		}
	case ai.RoleToolResult:
		writeLine(b, "[Tool result]: "+clipRunes(msg.Text, toolResultRunes))
	}
}

func writeLine(b *strings.Builder, line string) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(line)
}

func formatCalls(calls []ai.ToolCall) string {
	parts := make([]string, len(calls))
	for i, call := range calls {
		args := strings.TrimSpace(string(call.Arguments))
		if args == "" {
			args = "{}"
		}
		parts[i] = call.Name + "(" + args + ")"
	}
	return strings.Join(parts, "; ")
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + fmt.Sprintf("…[%d chars truncated]", len(r)-n)
}

func collectFiles(msgs []ai.Message, prevRead, prevMod []string) (read, modified []string) {
	read = append([]string(nil), prevRead...)
	modified = append([]string(nil), prevMod...)
	seenRead := stringSet(read)
	seenMod := stringSet(modified)
	for _, msg := range msgs {
		if msg.Role != ai.RoleAssistant {
			continue
		}
		for _, call := range msg.ToolCalls {
			path := toolPath(call.Arguments)
			if path == "" {
				continue
			}
			switch call.Name {
			case "read_file":
				if !seenRead[path] {
					seenRead[path] = true
					read = append(read, path)
				}
			case "write_file", "edit_file":
				if !seenMod[path] {
					seenMod[path] = true
					modified = append(modified, path)
				}
			}
		}
	}
	return read, modified
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

func toolPath(args []byte) string {
	if len(args) == 0 || !json.Valid(args) {
		return ""
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Path)
}

func appendFileLists(summary string, read, modified []string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(summary, "\n"))
	writePaths(&b, "read-files", read)
	writePaths(&b, "modified-files", modified)
	return b.String()
}

func writePaths(b *strings.Builder, tag string, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(b, "\n\n<%s>\n", tag)
	for _, path := range paths {
		b.WriteString(path)
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "</%s>", tag)
}

func stripFileTags(s string) string {
	s = removeTag(s, "read-files")
	s = removeTag(s, "modified-files")
	return strings.TrimSpace(s)
}

func removeTag(s, name string) string {
	open := "<" + name + ">"
	closeTag := "</" + name + ">"
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		rest := s[i:]
		j := strings.Index(rest, closeTag)
		if j < 0 {
			return strings.TrimSpace(s[:i])
		}
		s = s[:i] + s[i+j+len(closeTag):]
	}
}

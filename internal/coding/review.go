package coding

import (
	"context"
	"encoding/json"
	"os"
	"unicode/utf8"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

const (
	reviewDenied = "file change denied"
	reviewNeeded = "file change needs review"
)

// filePlan is one write or edit that can be shown before it is applied.
// note, when set, is why the old bytes are not in the diff.
type filePlan struct {
	abs     string
	path    string
	before  string
	after   string
	mode    os.FileMode
	existed bool
	note    string
	kind    string
}

// reviewFile asks before write_file and edit_file change the workspace.
// A call that cannot apply falls through, and Execute reports the error.
// Identical bytes are not a question. A nil confirm denies, because -p
// and the line session have no screen to ask on.
func (s *Session) reviewFile(ctx context.Context, call ai.ToolCall) (agent.Decision, error) {
	plan, ok := reviewPlan(s.ws, call)
	if !ok {
		return agent.Decision{}, nil
	}
	if s.confirm == nil {
		return agent.Decision{Block: true, Reason: reviewNeeded}, nil
	}
	allow, err := s.confirm(ctx, Confirm{
		Title:  "! allow this " + plan.kind + "?",
		Body:   planText(plan),
		Detail: plan.path,
	})
	if err != nil {
		return agent.Decision{}, err
	}
	if !allow {
		return agent.Decision{Block: true, Reason: reviewDenied}, nil
	}
	return agent.Decision{}, nil
}

// reviewPlan reports ok when the call would change a file and can be shown.
// The preview does not write. Execute applies the call again after an allow,
// so a file that changes in between can make an edit miss.
func reviewPlan(ws *Workspace, call ai.ToolCall) (filePlan, bool) {
	switch call.Name {
	case editToolName:
		var args editArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return filePlan{}, false
		}
		plan, err := prepareEdit(ws, args)
		if err != nil {
			return filePlan{}, false
		}
		plan.kind = "edit"
		return plan, true
	case writeToolName:
		var args writeArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return filePlan{}, false
		}
		plan, err := prepareWrite(ws, args)
		if err != nil {
			return filePlan{}, false
		}
		// Same bytes are not a decision. Execute still writes.
		if plan.existed && plan.note == "" && plan.before == plan.after {
			return filePlan{}, false
		}
		plan.kind = "write"
		return plan, true
	default:
		return filePlan{}, false
	}
}

func planText(plan filePlan) string {
	if plan.note != "" {
		return notedPreview(plan)
	}
	if !plan.existed && plan.after == "" {
		return "create " + plan.path + "\n(empty file)"
	}
	return unifiedDiff(plan.path, plan.before, plan.after, plan.existed)
}

func notedPreview(plan filePlan) string {
	if plan.after == "" {
		return plan.note + "\n(empty file)"
	}
	if !utf8.ValidString(plan.after) {
		return plan.note
	}
	return plan.note + "\n" + unifiedDiff(plan.path, "", plan.after, false)
}

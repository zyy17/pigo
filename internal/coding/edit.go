package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

const editToolName = "edit_file"

type editArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func editTool(ws *Workspace) agent.Tool {
	return newTool(editToolName,
		"Make precise edits by replacing old_string with new_string. old_string must match exactly once unless replace_all is true.",
		ai.ObjectSchema(map[string]ai.Property{
			"path":        ai.StringProperty("Path to the file to edit (relative or absolute)"),
			"old_string":  ai.StringProperty("Exact text to replace. Keep it small and unique"),
			"new_string":  ai.StringProperty("Replacement text"),
			"replace_all": ai.BoolProperty("Replace every match. Default false"),
		}, "path", "old_string", "new_string"),
		func(_ context.Context, args editArgs) (string, error) {
			return editFile(ws, args)
		},
	)
}

func editFile(ws *Workspace, args editArgs) (string, error) {
	plan, err := prepareEdit(ws, args)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(plan.abs, []byte(plan.after), plan.mode); err != nil {
		return "", err
	}
	return fmt.Sprintf("updated %s", args.Path), nil
}

// prepareEdit checks the call and builds the next text. It does not write.
func prepareEdit(ws *Workspace, args editArgs) (filePlan, error) {
	if err := requirePath(args.Path); err != nil {
		return filePlan{}, err
	}
	if args.OldString == "" {
		return filePlan{}, errors.New("old_string is empty")
	}
	if args.OldString == args.NewString {
		return filePlan{}, errors.New("old_string and new_string are the same")
	}
	abs, err := ws.Resolve(args.Path)
	if err != nil {
		return filePlan{}, err
	}
	info, err := statFile(args.Path, abs)
	if err != nil {
		return filePlan{}, err
	}
	if info.Size() > maxFileBytes {
		return filePlan{}, fmt.Errorf("%s is too large to edit", args.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return filePlan{}, err
	}
	before := string(data)
	count := strings.Count(before, args.OldString)
	if count == 0 {
		return filePlan{}, errors.New("old_string was not found")
	}
	if count > 1 && !args.ReplaceAll {
		return filePlan{}, fmt.Errorf("old_string matched %d times; include more context or set replace_all", count)
	}
	after := strings.Replace(before, args.OldString, args.NewString, 1)
	if args.ReplaceAll {
		after = strings.ReplaceAll(before, args.OldString, args.NewString)
	}
	plan := filePlan{
		abs:     abs,
		path:    args.Path,
		before:  before,
		after:   after,
		mode:    info.Mode().Perm(),
		existed: true,
	}
	if !utf8.ValidString(before) {
		plan.note = args.Path + " is not valid UTF-8, so this review shows the new content only"
	}
	return plan, nil
}

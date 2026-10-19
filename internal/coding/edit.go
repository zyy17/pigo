package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

type editArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func editTool(ws *Workspace) agent.Tool {
	return newTool("edit_file",
		"Make precise edits by replacing old_string with new_string. old_string must match exactly once unless replace_all is true.",
		ai.ObjectSchema(map[string]ai.Property{
			"path":        ai.StringProperty("Path to the file to edit (relative or absolute)"),
			"old_string":  ai.StringProperty("Exact text to replace. Keep it small and unique"),
			"new_string":  ai.StringProperty("Replacement text"),
			"replace_all": ai.BoolProperty("Replace every match. Default false"),
		}, "path", "old_string", "new_string"),
		func(_ context.Context, args editArgs) (string, error) {
			return editFile(ws, args.Path, args.OldString, args.NewString, args.ReplaceAll)
		},
	)
}

func editFile(ws *Workspace, path, old, replacement string, replaceAll bool) (string, error) {
	if err := requirePath(path); err != nil {
		return "", err
	}
	if old == "" {
		return "", errors.New("old_string is empty")
	}
	if old == replacement {
		return "", errors.New("old_string and new_string are the same")
	}
	abs, err := ws.Resolve(path)
	if err != nil {
		return "", err
	}
	info, err := statFile(path, abs)
	if err != nil {
		return "", err
	}
	if info.Size() > maxFileBytes {
		return "", fmt.Errorf("%s is too large to edit", path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	content := string(data)
	count := strings.Count(content, old)
	if count == 0 {
		return "", fmt.Errorf("old_string was not found")
	}
	if count > 1 && !replaceAll {
		return "", fmt.Errorf("old_string matched %d times; include more context or set replace_all", count)
	}
	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, old, replacement)
	} else {
		updated = strings.Replace(content, old, replacement, 1)
	}
	if err := os.WriteFile(abs, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return fmt.Sprintf("updated %s", path), nil
}

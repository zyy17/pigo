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

func editTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:  "edit_file",
		Label: "Edit file",
		Description: "Replace old_string with new_string in a workspace file. " +
			"old_string must match exactly once unless replace_all is true.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"path":        ai.StringProp("File path relative to the workspace"),
			"old_string":  ai.StringProp("Exact text to replace"),
			"new_string":  ai.StringProp("Replacement text"),
			"replace_all": ai.BoolProp("Replace every match. Default false"),
		}, "path", "old_string", "new_string"),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args struct {
				Path       string `json:"path"`
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := decodeArgs(raw, &args); err != nil {
				return "", err
			}
			return editFile(ctx, ws, args.Path, args.OldString, args.NewString, args.ReplaceAll)
		},
	}
}

func editFile(ctx context.Context, ws *Workspace, path, old, new string, replaceAll bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := needPath(path); err != nil {
		return "", err
	}
	if old == "" {
		return "", fmt.Errorf("old_string is empty")
	}
	if old == new {
		return "", fmt.Errorf("old_string and new_string are the same")
	}
	abs, err := ws.Resolve(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", notFound(path)
		}
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
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
	updated := content
	if replaceAll {
		updated = strings.ReplaceAll(content, old, new)
	} else {
		updated = strings.Replace(content, old, new, 1)
	}
	if err := os.WriteFile(abs, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return fmt.Sprintf("updated %s", path), nil
}

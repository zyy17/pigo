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

const maxListEntries = 500

func listTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:        "list_dir",
		Label:       "List directory",
		Description: "List one directory in the workspace. path defaults to the workspace root. Directories end with a slash.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"path": ai.StringProp("Directory path relative to the workspace. Defaults to the root"),
		}),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if len(strings.TrimSpace(string(raw))) == 0 {
				raw = []byte("{}")
			}
			if err := decodeArgs(raw, &args); err != nil {
				return "", err
			}
			return listDir(ctx, ws, args.Path)
		},
	}
}

func listDir(ctx context.Context, ws *Workspace, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		path = "."
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
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}
	shown := entries
	clipped := false
	if len(shown) > maxListEntries {
		shown = shown[:maxListEntries]
		clipped = true
	}
	var b strings.Builder
	for i, entry := range shown {
		if i > 0 {
			b.WriteByte('\n')
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		b.WriteString(name)
	}
	if clipped {
		fmt.Fprintf(&b, "\n…(%d more)", len(entries)-maxListEntries)
	}
	return b.String(), nil
}

package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

const maxListEntries = 500

type listArgs struct {
	Path string `json:"path"`
}

func listTool(ws *Workspace) agent.Tool {
	return newTool("list_dir",
		"List directory contents. Returns entries sorted alphabetically, with '/' suffix for directories. Includes dotfiles. Output is truncated to 500 entries.",
		ai.ObjectSchema(map[string]ai.Property{
			"path": ai.StringProperty("Directory to list (default: current directory)"),
		}),
		func(_ context.Context, args listArgs) (string, error) {
			return listDir(ws, args.Path)
		},
	)
}

func listDir(ws *Workspace, path string) (string, error) {
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
			return "", missingPath(path)
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
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	})
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

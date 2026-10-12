package coding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func writeTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:        "write_file",
		Label:       "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"path":    ai.StringProperty("Path to the file to write (relative or absolute)"),
			"content": ai.StringProperty("Content to write to the file"),
		}, "path", "content"),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args writeArgs
			if err := decodeArgs(raw, &args); err != nil {
				return "", err
			}
			return writeFile(ctx, ws, args.Path, args.Content)
		},
	}
}

func writeFile(ctx context.Context, ws *Workspace, path, content string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := requirePath(path); err != nil {
		return "", err
	}
	abs, err := ws.Resolve(path)
	if err != nil {
		return "", err
	}
	if abs == ws.Root() {
		return "", fmt.Errorf("%s is the workspace root", path)
	}
	info, err := os.Stat(abs)
	if err == nil && info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s (%d bytes)", path, len(content)), nil
}

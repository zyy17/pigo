package coding

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func readTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:        "read_file",
		Label:       "Read file",
		Description: "Read a UTF-8 text file in the workspace. path is relative to the workspace root. Lines are numbered.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"path": ai.StringProp("File path relative to the workspace"),
		}, "path"),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := decodeArgs(raw, &args); err != nil {
				return "", err
			}
			return readFile(ctx, ws, args.Path)
		},
	}
}

func readFile(ctx context.Context, ws *Workspace, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := needPath(path); err != nil {
		return "", err
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

	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return "", err
	}
	truncated := len(data) > maxFileBytes
	if truncated {
		data = data[:maxFileBytes]
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%s is not valid UTF-8 text", path)
	}
	text := numberLines(string(data))
	if truncated {
		text += "\n…(truncated)"
	}
	return text, nil
}

func numberLines(text string) string {
	if text == "" {
		return "(empty)"
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d|%s", i+1, line)
	}
	return b.String()
}

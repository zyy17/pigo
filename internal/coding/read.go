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

type readArgs struct {
	Path string `json:"path"`
}

func readTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:        "read_file",
		Label:       "read_file",
		Description: "Read the contents of a text file. Output is truncated. Lines are numbered. path is relative or absolute inside the workspace.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"path": ai.StringProperty("Path to the file to read (relative or absolute)"),
		}, "path"),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args readArgs
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
	if err := requirePath(path); err != nil {
		return "", err
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
	text := lineNumbers(string(data))
	if truncated {
		text += "\n…(truncated)"
	}
	return text, nil
}

func lineNumbers(text string) string {
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

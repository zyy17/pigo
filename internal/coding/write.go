package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

const writeToolName = "write_file"

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func writeTool(ws *Workspace) agent.Tool {
	return newTool(writeToolName,
		"Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		ai.ObjectSchema(map[string]ai.Property{
			"path":    ai.StringProperty("Path to the file to write (relative or absolute)"),
			"content": ai.StringProperty("Content to write to the file"),
		}, "path", "content"),
		func(_ context.Context, args writeArgs) (string, error) {
			return writeFile(ws, args)
		},
	)
}

func writeFile(ws *Workspace, args writeArgs) (string, error) {
	plan, err := prepareWrite(ws, args)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(plan.abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(plan.abs, []byte(plan.after), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s (%d bytes)", args.Path, len(args.Content)), nil
}

// prepareWrite checks the call and reads the old text when it is small
// enough to review. It does not create directories or write.
func prepareWrite(ws *Workspace, args writeArgs) (filePlan, error) {
	if err := requirePath(args.Path); err != nil {
		return filePlan{}, err
	}
	abs, err := ws.Resolve(args.Path)
	if err != nil {
		return filePlan{}, err
	}
	if abs == ws.Root() {
		return filePlan{}, fmt.Errorf("%s is the workspace root", args.Path)
	}
	plan := filePlan{abs: abs, path: args.Path, after: args.Content}
	info, err := os.Stat(abs)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return filePlan{}, err
		}
		return plan, nil
	}
	if info.IsDir() {
		return filePlan{}, fmt.Errorf("%s is a directory", args.Path)
	}
	plan.existed = true
	if info.Size() > maxFileBytes {
		plan.note = fmt.Sprintf("existing file is %d bytes, so this review shows the new content only", info.Size())
		return plan, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return filePlan{}, err
	}
	if !utf8.Valid(data) {
		plan.note = args.Path + " is not valid UTF-8, so this review shows the new content only"
		return plan, nil
	}
	plan.before = string(data)
	return plan, nil
}

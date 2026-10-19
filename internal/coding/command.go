package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

// commandTimeout bounds run_command. Tests may shorten it.
var commandTimeout = 30 * time.Second

// shellWaitDelay is how long a canceled shell may keep its pipes open.
const shellWaitDelay = 500 * time.Millisecond

const maxCommandRunes = 8_000

type commandArgs struct {
	Command string `json:"command"`
}

const runCommandTool = "run_command"

func commandTool(ws *Workspace) agent.Tool {
	return newTool(runCommandTool,
		"Execute a bash command in the current working directory. Returns stdout and stderr. Output is truncated. The timeout is 30 seconds.",
		ai.ObjectSchema(map[string]ai.Property{
			"command": ai.StringProperty("Bash command to execute"),
		}, "command"),
		func(ctx context.Context, args commandArgs) (string, error) {
			return runCommand(ctx, ws, args.Command)
		},
	)
}

func runCommand(ctx context.Context, ws *Workspace, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is empty")
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, commandTimeout)
	defer cancel()

	cmd := newShellCommand(ctx, command)
	cmd.Dir = ws.Root()
	cmd.Env = shellEnv(ws.Root())
	out, err := cmd.CombinedOutput()
	text := clipText(string(out), maxCommandRunes)

	if errors.Is(parent.Err(), context.Canceled) {
		return "", parent.Err()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("command timed out after %s\n%s", commandTimeout, text)
	}
	if err != nil {
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%w\n%s", err, text)
	}
	if strings.TrimSpace(text) == "" {
		return "(no output)", nil
	}
	return text, nil
}

// shellEnv hides OPENAI_API_KEY and points PWD at the workspace.
// The model can run commands, so the key must not be visible there.
func shellEnv(root string) []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "OPENAI_API_KEY="),
			strings.HasPrefix(kv, "PWD="),
			strings.HasPrefix(kv, "OLDPWD="):
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "PWD="+root)
	return out
}

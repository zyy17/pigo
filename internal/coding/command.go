package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

// commandTimeout bounds run_command. Tests may shorten it.
var commandTimeout = 30 * time.Second

type commandArgs struct {
	Command string `json:"command"`
}

const runCommandTool = "run_command"

func commandTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:  runCommandTool,
		Label: "run_command",
		Description: "Execute a bash command in the current working directory. " +
			"Returns stdout and stderr. Output is truncated. The timeout is 30 seconds.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"command": ai.StringProperty("Shell command to execute"),
		}, "command"),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args commandArgs
			if err := decodeArgs(raw, &args); err != nil {
				return "", err
			}
			return runCommand(ctx, ws, args.Command)
		},
	}
}

func runCommand(ctx context.Context, ws *Workspace, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("command is empty")
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, commandTimeout)
	defer cancel()

	cmd := newShellCommand(ctx, command)
	cmd.Dir = ws.Root()
	cmd.Env = shellEnv(ws.Root())
	out, err := cmd.CombinedOutput()
	text := clipText(string(out), 8_000)

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

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	// A shell child can keep stdout open after the shell exits.
	// configureShell kills the whole group and bounds that wait.
	configureShell(cmd)
	return cmd
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

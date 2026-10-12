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

func commandTool(ws *Workspace) agent.Tool {
	return agent.Tool{
		Name:  "run_command",
		Label: "Run command",
		Description: "Run a shell command with the workspace as the working directory. " +
			"The timeout is 30 seconds. Output is truncated.",
		Parameters: ai.ObjectSchema(map[string]ai.Property{
			"command": ai.StringProp("Shell command to run"),
		}, "command"),
		Execute: func(ctx context.Context, raw []byte) (string, error) {
			var args struct {
				Command string `json:"command"`
			}
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

	cmd := shellCommand(ctx, command)
	cmd.Dir = ws.Root()
	cmd.Env = commandEnv(ws.Root())
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

func shellCommand(ctx context.Context, command string) *exec.Cmd {
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

// commandEnv hides the API key and points PWD at the workspace.
func commandEnv(root string) []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "PIGO_API_KEY="),
			strings.HasPrefix(kv, "PWD="),
			strings.HasPrefix(kv, "OLDPWD="):
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "PWD="+root)
	return out
}

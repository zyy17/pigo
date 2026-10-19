//go:build windows

package coding

import (
	"context"
	"os/exec"
)

// configureShell bounds how long a canceled command may hold its pipes.
func configureShell(cmd *exec.Cmd) {
	cmd.WaitDelay = shellWaitDelay
}

func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd", "/C", command)
	configureShell(cmd)
	return cmd
}

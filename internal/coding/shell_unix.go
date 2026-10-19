//go:build unix

package coding

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureShell puts the shell in its own process group and kills that
// group when the command context ends. Otherwise "sleep 5" inside sh
// keeps the output pipe open and the tool waits for the child.
func configureShell(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = shellWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

// newShellCommand runs command in sh. configureShell kills the process
// group, because a child can keep stdout open after sh exits.
func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	configureShell(cmd)
	return cmd
}

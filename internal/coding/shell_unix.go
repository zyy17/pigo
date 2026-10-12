//go:build unix

package coding

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// configureShell puts the shell in its own process group and kills that
// group when the command context ends. Otherwise "sleep 5" inside sh
// keeps the output pipe open and the tool waits for the child.
func configureShell(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 500 * time.Millisecond
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

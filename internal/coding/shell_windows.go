//go:build windows

package coding

import (
	"os/exec"
	"time"
)

// configureShell bounds how long a canceled command may hold its pipes.
func configureShell(cmd *exec.Cmd) {
	cmd.WaitDelay = 500 * time.Millisecond
}

//go:build unix

package toolkit

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureBashCommandCancellation puts the shell in its own process group so
// CommandContext cancellation terminates descendants that inherited its output
// descriptors, not only the shell process itself.
func configureBashCommandCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

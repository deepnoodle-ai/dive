//go:build windows

package toolkit

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

// configureBashCommandCancellation replaces CommandContext's single-process
// kill with taskkill's process-tree termination on Windows.
func configureBashCommandCancellation(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := exec.Command(
			"taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F",
		).Run(); err == nil {
			return nil
		}
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return os.ErrProcessDone
		}
		return err
	}
}

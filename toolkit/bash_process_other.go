//go:build !unix && !windows

package toolkit

import "os/exec"

// configureBashCommandCancellation retains CommandContext's default
// single-process cancellation on platforms without process-group support.
func configureBashCommandCancellation(_ *exec.Cmd) {}

//go:build !unix && !windows

package procgroup

import "os/exec"

// ConfigureCancellation retains CommandContext's default
// single-process cancellation on platforms without process-group support.
func ConfigureCancellation(_ *exec.Cmd) {}

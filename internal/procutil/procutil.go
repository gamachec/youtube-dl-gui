// Package procutil runs external processes without a console window and
// kills their whole process tree on cancellation.
package procutil

import (
	"context"
	"os"
	"os/exec"
)

// Command returns a Cmd configured for background use: no console window on
// Windows, UTF-8 Python I/O, and cancellation of ctx kills the whole process
// tree (not only the parent, which exec.CommandContext alone would do).
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	cmd.Cancel = func() error { return KillTree(cmd) }
	hideWindow(cmd)
	return cmd
}

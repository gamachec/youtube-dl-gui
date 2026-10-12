//go:build windows

package procutil

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// KillTree terminates the process and all its children using taskkill.
func KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Plain exec.Command: the context may already be cancelled.
	kill := exec.CommandContext(context.Background(), "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	hideWindow(kill)
	if out, err := kill.CombinedOutput(); err != nil {
		// Fall back to killing the parent only.
		if kerr := cmd.Process.Kill(); kerr != nil {
			return fmt.Errorf("taskkill: %w (%s)", err, out)
		}
	}
	return nil
}

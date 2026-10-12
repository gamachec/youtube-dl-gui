//go:build !windows

package procutil

import (
	"fmt"
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	// Own process group so that KillTree can signal the children too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// KillTree terminates the process and its process group.
func KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if kerr := cmd.Process.Kill(); kerr != nil {
			return fmt.Errorf("kill process: %w", kerr)
		}
	}
	return nil
}

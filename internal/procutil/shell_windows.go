//go:build windows

package procutil

import (
	"fmt"
	"os/exec"
	"syscall"
)

// start launches cmd without waiting; the process is reaped in the background.
// Windows are deliberately NOT hidden here: these helpers open user-facing UI.
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmd.Path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ShowInFolder opens Explorer with path selected.
func ShowInFolder(path string) error {
	cmd := exec.Command("explorer.exe")
	// Raw command line: Go's argv quoting would wrap "/select,path" as a whole,
	// which Explorer does not understand.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	return start(cmd)
}

// OpenDir opens a folder in Explorer.
func OpenDir(dir string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe "` + dir + `"`}
	return start(cmd)
}

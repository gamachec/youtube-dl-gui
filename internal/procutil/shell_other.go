//go:build !windows

package procutil

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmd.Path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ShowInFolder opens the parent folder of path.
func ShowInFolder(path string) error { return OpenDir(filepath.Dir(path)) }

// OpenDir opens a folder with the desktop's default handler.
func OpenDir(dir string) error { return start(exec.Command("xdg-open", dir)) }

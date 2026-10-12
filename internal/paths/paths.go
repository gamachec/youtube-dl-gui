// Package paths resolves the portable directory layout used at runtime.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appDirName = "youtube-dl-gui"

// Paths holds every directory and file location used by the application.
type Paths struct {
	Root         string
	Bin          string
	Logs         string
	ConfigFile   string
	VersionsFile string
}

// Resolve locates the portable root next to the executable, falling back to
// %LOCALAPPDATA%\youtube-dl-gui when that folder is not writable.
func Resolve() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, fmt.Errorf("locate executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return resolve(filepath.Dir(exe), isWritable, fallbackRoot)
}

// resolve is the testable core of Resolve.
func resolve(exeDir string, writable func(dir string) bool, fallback func() (string, error)) (Paths, error) {
	root := exeDir
	if !writable(root) {
		fb, err := fallback()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve fallback root: %w", err)
		}
		root = fb
	}
	p := New(root)
	for _, dir := range []string{p.Bin, p.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Paths{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return p, nil
}

// New builds a Paths value for the given root without touching the disk.
func New(root string) Paths {
	bin := filepath.Join(root, "bin")
	return Paths{
		Root:         root,
		Bin:          bin,
		Logs:         filepath.Join(root, "logs"),
		ConfigFile:   filepath.Join(root, "config.json"),
		VersionsFile: filepath.Join(bin, "versions.json"),
	}
}

// Tool returns the full path of an external tool inside Bin
// (".exe" is appended on Windows only).
func (p Paths) Tool(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(p.Bin, name)
}

func fallbackRoot() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		if base, err = os.UserConfigDir(); err != nil {
			return "", fmt.Errorf("no local app data directory: %w", err)
		}
	}
	return filepath.Join(base, appDirName), nil
}

// isWritable checks that a file can be created and removed in dir.
func isWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	return os.Remove(name) == nil
}

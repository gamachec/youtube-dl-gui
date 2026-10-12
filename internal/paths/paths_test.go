package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWritable(t *testing.T) {
	exeDir := t.TempDir()
	p, err := resolve(exeDir, func(string) bool { return true }, func() (string, error) {
		t.Fatal("fallback must not be used")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Root != exeDir {
		t.Errorf("Root = %q, want %q", p.Root, exeDir)
	}
	for _, d := range []string{p.Bin, p.Logs} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Errorf("directory %q not created", d)
		}
	}
}

func TestResolveFallback(t *testing.T) {
	fb := filepath.Join(t.TempDir(), "fallback")
	p, err := resolve(t.TempDir(), func(string) bool { return false }, func() (string, error) { return fb, nil })
	if err != nil {
		t.Fatal(err)
	}
	if p.Root != fb {
		t.Errorf("Root = %q, want %q", p.Root, fb)
	}
	if _, err := os.Stat(p.Bin); err != nil {
		t.Errorf("bin not created: %v", err)
	}
}

func TestLayout(t *testing.T) {
	p := New(filepath.Join("r"))
	if p.ConfigFile != filepath.Join("r", "config.json") || p.VersionsFile != filepath.Join("r", "bin", "versions.json") {
		t.Errorf("unexpected layout: %+v", p)
	}
	if got := filepath.Dir(p.Tool("yt-dlp")); got != p.Bin {
		t.Errorf("Tool dir = %q, want %q", got, p.Bin)
	}
}

func TestIsWritable(t *testing.T) {
	if !isWritable(t.TempDir()) {
		t.Error("temp dir should be writable")
	}
	if isWritable(filepath.Join(t.TempDir(), "missing")) {
		t.Error("missing dir should not be writable")
	}
}

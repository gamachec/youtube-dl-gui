package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := Config{OutputDir: "/some/dir", LastMode: "audio", Version: 1}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temporary file should not remain")
	}
}

func TestLoadMissing(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Errorf("got %+v, want defaults %+v", got, Default())
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestUsableOutputDir(t *testing.T) {
	dir := t.TempDir()
	if got, warn := UsableOutputDir(dir); got != dir || warn != "" {
		t.Errorf("existing dir: got %q, %q", got, warn)
	}
	got, warn := UsableOutputDir(filepath.Join(dir, "missing"))
	if got == "" || warn == "" {
		t.Errorf("missing dir: got %q, %q; want fallback and warning", got, warn)
	}
}

package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRotates(t *testing.T) {
	dir := t.TempDir()
	for _, msg := range []string{"first", "second"} {
		closeFn, err := Init(dir)
		if err != nil {
			t.Fatal(err)
		}
		slog.Info(msg)
		closeFn()
	}
	cur, _ := os.ReadFile(filepath.Join(dir, "app.log"))
	prev, _ := os.ReadFile(filepath.Join(dir, "app.prev.log"))
	if !strings.Contains(string(cur), "second") || strings.Contains(string(cur), "first") {
		t.Errorf("app.log = %q", cur)
	}
	if !strings.Contains(string(prev), "first") {
		t.Errorf("app.prev.log = %q", prev)
	}
}

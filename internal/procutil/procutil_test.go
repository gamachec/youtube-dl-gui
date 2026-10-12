//go:build !windows

package procutil

import (
	"context"
	"testing"
	"time"
)

func TestCommandCancelKillsTree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// The shell spawns a child sleep: both must die.
	cmd := Command(ctx, "sh", "-c", "sleep 30 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	time.AfterFunc(100*time.Millisecond, cancel)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after kill")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process not terminated after cancel")
	}
}

func TestCommandEnv(t *testing.T) {
	out, err := Command(context.Background(), "sh", "-c", "echo $PYTHONUTF8").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "1\n" {
		t.Fatalf("PYTHONUTF8 = %q", out)
	}
}

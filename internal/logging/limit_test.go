package logging

import (
	"bytes"
	"testing"
)

func TestLimitWriter(t *testing.T) {
	var buf bytes.Buffer
	l := &limitWriter{w: &buf, left: 5}
	for _, in := range []string{"abc", "defg", "hij"} {
		if n, err := l.Write([]byte(in)); err != nil || n != len(in) {
			t.Fatalf("Write(%q) = %d, %v", in, n, err)
		}
	}
	if got := buf.String(); got != "abcde" {
		t.Errorf("got %q, want %q", got, "abcde")
	}
}

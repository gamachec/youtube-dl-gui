// Package logging initialises slog to logs/app.log.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Init redirects the default slog logger to <logsDir>/app.log. The previous
// run's log is kept as app.prev.log. The returned func closes the file.
func Init(logsDir string) (func() error, error) {
	logPath := filepath.Join(logsDir, "app.log")
	if _, err := os.Stat(logPath); err == nil {
		if err := os.Rename(logPath, filepath.Join(logsDir, "app.prev.log")); err != nil {
			return nil, fmt.Errorf("rotate log: %w", err)
		}
	}
	f, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log: %w", err)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(&limitWriter{w: f, left: maxLogSize}, nil)))
	return f.Close, nil
}

// maxLogSize bounds one run's log; the previous run is kept separately.
const maxLogSize = 5 << 20

// limitWriter silently drops everything written after left bytes.
type limitWriter struct {
	w    io.Writer
	mu   sync.Mutex
	left int
}

func (l *limitWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.left <= 0 {
		return len(b), nil
	}
	n := len(b)
	if n > l.left {
		b = b[:l.left]
	}
	l.left -= len(b)
	if _, err := l.w.Write(b); err != nil {
		return 0, err
	}
	return n, nil
}

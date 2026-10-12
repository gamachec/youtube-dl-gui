package ytdlp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cgamache/youtube-dl-gui/internal/procutil"
)

// ErrCancelled is returned when the download context is cancelled.
var ErrCancelled = errors.New("téléchargement annulé")

const (
	stderrKeep    = 8 * 1024
	eventInterval = 100 * time.Millisecond // ~10 events/s
)

// tailBuffer keeps the last stderrKeep bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > stderrKeep {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-stderrKeep:]...)
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// checkOutputDir verifies that dir exists and is writable.
func checkOutputDir(dir string) error {
	st, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("le dossier de sortie est introuvable (%s): %w", dir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("le chemin de sortie n'est pas un dossier : %s", dir)
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("le dossier de sortie n'est pas accessible en écriture (%s): %w", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// Download runs yt-dlp for req, reporting progress through onEvent (may be
// nil), and returns the path of the final file.
func (r Runner) Download(ctx context.Context, req Request, onEvent func(Event)) (string, error) {
	if !req.Mode.Valid() {
		return "", fmt.Errorf("mode de téléchargement inconnu : %q", req.Mode)
	}
	if err := checkOutputDir(req.OutputDir); err != nil {
		return "", err
	}
	if onEvent == nil {
		onEvent = func(Event) {}
	}

	args := BuildArgs(req, BinPaths{FFmpegDir: r.FFmpegDir, Deno: r.Deno})
	cmd := procutil.Command(ctx, r.YtDlp, args...)
	var stderr tailBuffer
	cmd.Stderr = io.MultiWriter(&stderr, slogWriter{"stderr"})
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("lancement de yt-dlp impossible: %w", err)
	}

	p := newParser(req.Mode)
	var last time.Time
	var lastPhase string
	var pending *Event
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		slog.Debug("yt-dlp", "stdout", line)
		ev, ok := p.feed(line)
		if !ok {
			continue
		}
		// Throttle progress ticks; phase changes always go through.
		if ev.Phase != lastPhase || time.Since(last) >= eventInterval {
			onEvent(ev)
			last, lastPhase, pending = time.Now(), ev.Phase, nil
		} else {
			pending = &ev
		}
	}
	if pending != nil {
		onEvent(*pending)
	}
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		cleanupPartial(req.OutputDir, p.videoID)
		return "", ErrCancelled
	}
	if waitErr != nil {
		return "", errors.New(MapError(stderr.String()))
	}
	if p.file == "" {
		return "", errors.New("yt-dlp n'a pas indiqué le fichier produit")
	}
	onEvent(Event{Phase: p.phase, Percent: 100})
	return p.file, nil
}

type slogWriter struct{ stream string }

func (w slogWriter) Write(b []byte) (int, error) {
	slog.Debug("yt-dlp", w.stream, strings.TrimRight(string(b), "\r\n"))
	return len(b), nil
}

var formatFragment = regexp.MustCompile(`\.f[A-Za-z0-9_-]+\.[A-Za-z0-9]+(\.part)?$`)

// cleanupPartial removes leftovers of an interrupted download of videoID.
func cleanupPartial(dir, videoID string) {
	if videoID == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	marker := "[" + videoID + "]"
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.Contains(n, marker) {
			continue
		}
		if strings.HasSuffix(n, ".part") || strings.HasSuffix(n, ".ytdl") ||
			strings.Contains(n, ".temp.") || formatFragment.MatchString(n) {
			if err := os.Remove(filepath.Join(dir, n)); err != nil {
				slog.Warn("cleanup partial file", "file", n, "err", err)
			}
		}
	}
}

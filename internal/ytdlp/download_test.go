package ytdlp

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain doubles as a fake yt-dlp when FAKE_YTDLP is set.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_YTDLP") != "" {
		fakeYtDlp()
		return
	}
	os.Exit(m.Run())
}

func fakeYtDlp() {
	dir := "."
	for i, a := range os.Args {
		if a == "-P" && i+1 < len(os.Args) {
			dir = os.Args[i+1]
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "Clip [dQw4w9WgXcQ].f401.mp4.part"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Other [aaaaaaaaaaa].f401.mp4.part"), []byte("x"), 0o644)
	os.Stdout.WriteString("[download] Destination: " + dir + "/Clip [dQw4w9WgXcQ].f401.mp4\n")
	os.Stdout.WriteString("PROG|downloading|10|100|NA|1024.0|5|401\n")
	time.Sleep(time.Minute)
}

func TestBuildArgs(t *testing.T) {
	bin := BinPaths{FFmpegDir: "/bin", Deno: "/bin/deno"}
	tests := []struct {
		mode Mode
		want []string
	}{
		{ModeVideoAudio, []string{"-f", "bv*+ba/b", "--merge-output-format", "mp4", "--postprocessor-args", mergeAudioArgs}},
		{ModeVideoOnly, []string{"-f", "bv*", "--remux-video", "mp4"}},
		{ModeAudioOnly, []string{"-f", "ba/b", "-x", "--audio-format", "mp3", "--audio-quality", "0"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			got := BuildArgs(Request{URL: "-https://x", OutputDir: "/out", Mode: tt.mode}, bin)
			if n := len(got); got[n-2] != "--" || got[n-1] != "-https://x" {
				t.Errorf("URL must follow --: %v", got)
			}
			rest := got[len(got)-2-len(tt.want) : len(got)-2]
			if !slices.Equal(rest, tt.want) {
				t.Errorf("mode args = %v, want %v", rest, tt.want)
			}
			for _, must := range []string{"--no-playlist", "--windows-filenames", "--no-mtime", "--no-quiet", "-P", "/out", "deno:/bin/deno"} {
				if !slices.Contains(got, must) {
					t.Errorf("missing %q in %v", must, got)
				}
			}
		})
	}
}

func TestParserFixtures(t *testing.T) {
	tests := []struct {
		file       string
		mode       Mode
		wantFile   string
		wantPhases []string
	}{
		{"output_video_audio.txt", ModeVideoAudio, "/out/Clip [dQw4w9WgXcQ].mp4",
			[]string{phaseVideo, phaseAudio, phaseMerge}},
		{"output_audio.txt", ModeAudioOnly, "/out/Clip [dQw4w9WgXcQ].mp3",
			[]string{phaseAudio, phaseMP3}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			p := newParser(tt.mode)
			var phases []string
			prev := -1.0
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				ev, ok := p.feed(sc.Text())
				if !ok {
					continue
				}
				if ev.Percent < prev {
					t.Errorf("percent went backwards: %v -> %v", prev, ev.Percent)
				}
				prev = ev.Percent
				if len(phases) == 0 || phases[len(phases)-1] != ev.Phase {
					phases = append(phases, ev.Phase)
				}
			}
			if p.file != tt.wantFile {
				t.Errorf("file = %q, want %q", p.file, tt.wantFile)
			}
			if p.videoID != "dQw4w9WgXcQ" {
				t.Errorf("videoID = %q", p.videoID)
			}
			if !slices.Equal(phases, tt.wantPhases) {
				t.Errorf("phases = %v, want %v", phases, tt.wantPhases)
			}
			if prev < 95 {
				t.Errorf("final percent = %v, want >= 95", prev)
			}
		})
	}
}

func TestParseProgressValues(t *testing.T) {
	p := newParser(ModeVideoOnly)
	ev, ok := p.feed("PROG|downloading|50|NA|100|2097152.0|90|137")
	if !ok || ev.Total != 100 || ev.Downloaded != 50 || ev.Speed != "2.0 Mio/s" || ev.ETA != "01:30" {
		t.Errorf("unexpected event %+v ok=%v", ev, ok)
	}
	if ev.Percent < 47 || ev.Percent > 48 { // 50% of the 95% download window
		t.Errorf("percent = %v", ev.Percent)
	}
	if _, ok := p.feed("PROG|downloading|NA"); ok {
		t.Error("short line should be ignored")
	}
}

func TestDownloadCancel(t *testing.T) {
	t.Setenv("FAKE_YTDLP", "1")
	dir := t.TempDir()
	r := Runner{YtDlp: os.Args[0]}
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := r.Download(ctx, Request{URL: "https://x", OutputDir: dir, Mode: ModeVideoAudio}, func(e Event) {
			if strings.HasPrefix(e.Phase, "Téléchargement") {
				select {
				case got <- struct{}{}:
				default:
				}
			}
		})
		done <- err
	}()
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("no event received")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("err = %v, want ErrCancelled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("download did not stop after cancel")
	}
	if _, err := os.Stat(filepath.Join(dir, "Clip [dQw4w9WgXcQ].f401.mp4.part")); !os.IsNotExist(err) {
		t.Error("partial file of the video should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "Other [aaaaaaaaaaa].f401.mp4.part")); err != nil {
		t.Error("other videos' files must be kept")
	}
}

func TestCheckOutputDir(t *testing.T) {
	if err := checkOutputDir(t.TempDir()); err != nil {
		t.Errorf("valid dir: %v", err)
	}
	if err := checkOutputDir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing dir should fail")
	}
}

package deps

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cgamache/youtube-dl-gui/internal/github"
	"github.com/cgamache/youtube-dl-gui/internal/paths"
)

func TestCompareYtdlp(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2025.10.22", "2025.10.22", 0},
		{"2025.10.22", "2025.10.22.1", -1},
		{"2025.10.22.1", "2025.10.22", 1},
		{"2025.9.30", "2025.10.1", -1},
		{"2025.10.22.0", "2025.10.22", 0},
		{"2026.1.1", "2025.12.31", 1},
	}
	for _, c := range cases {
		if got := CompareYtdlp(c.a, c.b); got != c.want {
			t.Errorf("CompareYtdlp(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompareSemver(t *testing.T) {
	if compareSemver("v2.5.4", "v2.5.10") != -1 || compareSemver("v2.5.4", "2.5.4") != 0 ||
		compareSemver("v2.6.0-rc.1", "v2.5.9") != 1 {
		t.Error("unexpected semver comparison")
	}
}

func TestNewerTimestamp(t *testing.T) {
	if !newerTimestamp("2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z") {
		t.Error("expected newer")
	}
	if newerTimestamp("2025-01-02T00:00:00Z", "2025-01-01T00:00:00Z") {
		t.Error("expected not newer")
	}
	if !newerTimestamp("garbage", "2025-01-01T00:00:00Z") {
		t.Error("unparseable should count as different")
	}
}

func TestParseChecksums(t *testing.T) {
	in := "ABC123  yt-dlp.exe\ndef456 *other.zip\n\n"
	sums, err := ParseChecksums(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if sums["yt-dlp.exe"] != "abc123" || sums["other.zip"] != "def456" {
		t.Errorf("got %v", sums)
	}
	bare, _ := ParseChecksums(strings.NewReader("deadbeef\n"))
	if h, ok := lookupChecksum(bare, "whatever"); !ok || h != "deadbeef" {
		t.Errorf("bare hash lookup failed: %v", bare)
	}
}

// ---- Ensure tests ----

type fakeRelease struct {
	tag, published string
	files          map[string][]byte // asset name -> content
	badSum         bool
}

type env struct {
	srv      *httptest.Server
	paths    paths.Paths
	releases map[string]*fakeRelease // "owner/repo"
	offline  bool
	versions map[string]string // binary base name -> output of --version
	now      time.Time
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func zipOf(entries map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{
		paths:    paths.New(t.TempDir()),
		releases: map[string]*fakeRelease{},
		versions: map[string]string{},
		now:      time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
	}
	os.MkdirAll(e.paths.Bin, 0o755)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		if e.offline {
			http.Error(w, "limit", http.StatusForbidden)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/repos/"), "/")
		rel := e.releases[parts[0]+"/"+parts[1]]
		if rel == nil {
			http.NotFound(w, r)
			return
		}
		var assets []string
		for name, data := range rel.files {
			assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":"%s/dl/%s/%s/%s","size":%d}`,
				name, e.srv.URL, parts[0], parts[1], name, len(data)))
		}
		fmt.Fprintf(w, `{"tag_name":%q,"published_at":%q,"assets":[%s]}`, rel.tag, rel.published, strings.Join(assets, ","))
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/dl/"), "/")
		w.Write(e.releases[parts[0]+"/"+parts[1]].files[parts[2]])
	})
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) setYtdlp(tag, body string) {
	data := []byte(body)
	e.versions["yt-dlp.exe"] = tag
	e.releases["yt-dlp/yt-dlp"] = &fakeRelease{tag: tag, files: map[string][]byte{
		"yt-dlp.exe":   data,
		"SHA2-256SUMS": []byte(sum(data) + "  yt-dlp.exe\n"),
	}}
}

func (e *env) setFFmpeg(published string, entries map[string]string, badSum bool) {
	z := zipOf(entries)
	s := sum(z)
	if badSum {
		s = strings.Repeat("0", 64)
	}
	e.releases["yt-dlp/FFmpeg-Builds"] = &fakeRelease{tag: "latest", published: published, files: map[string][]byte{
		"ffmpeg-master-latest-win64-gpl.zip": z,
		"checksums.sha256":                   []byte(s + "  ffmpeg-master-latest-win64-gpl.zip\n"),
	}}
}

func (e *env) setDeno(tag string) {
	z := zipOf(map[string]string{"deno.exe": "deno " + tag})
	e.releases["denoland/deno"] = &fakeRelease{tag: tag, files: map[string][]byte{
		"deno-x86_64-pc-windows-msvc.zip":           z,
		"deno-x86_64-pc-windows-msvc.zip.sha256sum": []byte(sum(z) + "\n"),
	}}
}

func (e *env) manager() *Manager {
	return &Manager{
		Paths: e.paths,
		GH:    &github.Client{BaseURL: e.srv.URL},
		Tools: DefaultTools(),
		VersionOf: func(_ context.Context, bin string) (string, error) {
			v, ok := e.versions[filepath.Base(bin)]
			if !ok {
				return "", errors.New("no such binary")
			}
			return v, nil
		},
		Now: func() time.Time { return e.now },
	}
}

func (e *env) seedAll() {
	e.setYtdlp("2025.10.22", "ytdlp-bin")
	e.setFFmpeg("2025-10-30T00:00:00Z", map[string]string{
		"ffmpeg-master-latest-win64-gpl/bin/ffmpeg.exe":  "ffmpeg",
		"ffmpeg-master-latest-win64-gpl/bin/ffprobe.exe": "ffprobe",
		"ffmpeg-master-latest-win64-gpl/LICENSE.txt":     "lic",
	}, false)
	e.setDeno("v2.5.4")
}

func readBin(t *testing.T, e *env, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.paths.Bin, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestEnsureFreshInstall(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	var steps []string
	st, err := e.manager().Ensure(context.Background(), func(p Progress) { steps = append(steps, p.Tool+":"+p.Step) })
	if err != nil {
		t.Fatal(err)
	}
	for f, want := range map[string]string{"yt-dlp.exe": "ytdlp-bin", "ffmpeg.exe": "ffmpeg", "ffprobe.exe": "ffprobe", "deno.exe": "deno v2.5.4"} {
		if got := readBin(t, e, f); got != want {
			t.Errorf("%s = %q want %q", f, got, want)
		}
	}
	if st.Versions["yt-dlp"] != "2025.10.22" || st.Versions["deno"] != "v2.5.4" || len(st.Warnings) != 0 {
		t.Errorf("status = %+v", st)
	}
	if _, err := os.Stat(e.paths.VersionsFile); err != nil {
		t.Error("versions.json missing")
	}
	if len(steps) == 0 || steps[0] != "yt-dlp:check" {
		t.Errorf("unexpected steps %v", steps)
	}
	if entries, _ := filepath.Glob(filepath.Join(e.paths.Bin, ".dl-*")); len(entries) != 0 {
		t.Errorf("temp dirs left: %v", entries)
	}
}

func TestEnsureUpToDateAndUpdate(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	m := e.manager()
	if _, err := m.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	// Same version: binary untouched.
	os.WriteFile(filepath.Join(e.paths.Bin, "yt-dlp.exe"), []byte("local"), 0o755)
	if _, err := m.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if readBin(t, e, "yt-dlp.exe") != "local" {
		t.Error("up-to-date tool was reinstalled")
	}

	// New patch release of yt-dlp.
	e.setYtdlp("2025.10.22.1", "ytdlp-new")
	st, err := m.Ensure(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if readBin(t, e, "yt-dlp.exe") != "ytdlp-new" || st.Versions["yt-dlp"] != "2025.10.22.1" {
		t.Errorf("update not applied: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(e.paths.Bin, "yt-dlp.exe.old")); err == nil {
		t.Error(".old backup left behind")
	}
}

func TestEnsureWeeklyCheck(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	m := e.manager()
	if _, err := m.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	e.setDeno("v2.6.0")

	e.now = e.now.Add(24 * time.Hour) // < 1 week: deno not even checked
	if st, _ := m.Ensure(context.Background(), nil); st.Versions["deno"] != "v2.5.4" {
		t.Errorf("deno checked too early: %v", st.Versions)
	}
	e.now = e.now.Add(8 * 24 * time.Hour)
	if st, _ := m.Ensure(context.Background(), nil); st.Versions["deno"] != "v2.6.0" {
		t.Errorf("deno not updated after a week: %v", st.Versions)
	}
}

func TestEnsureOffline(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	m := e.manager()
	if _, err := m.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	e.srv.Close()
	e.now = e.now.Add(30 * 24 * time.Hour)
	st, err := m.Ensure(context.Background(), nil)
	if err != nil {
		t.Fatalf("offline with local tools must succeed: %v", err)
	}
	if len(st.Warnings) != 3 || st.Versions["yt-dlp"] != "2025.10.22" {
		t.Errorf("status = %+v", st)
	}

	// Without local tool: blocking error.
	e2 := newEnv(t)
	e2.seedAll()
	m2 := e2.manager()
	e2.srv.Close()
	if _, err := m2.Ensure(context.Background(), nil); !errors.Is(err, ErrMissingTool) {
		t.Errorf("err = %v, want ErrMissingTool", err)
	}
}

func TestEnsureRateLimited(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	m := e.manager()
	m.GH = rateLimited{}
	if _, err := m.Ensure(context.Background(), nil); !errors.Is(err, ErrMissingTool) || !errors.Is(err, github.ErrRateLimited) {
		t.Errorf("err = %v", err)
	}
}

type rateLimited struct{}

func (rateLimited) LatestRelease(context.Context, string, string) (github.Release, error) {
	return github.Release{}, github.ErrRateLimited
}
func (rateLimited) Download(context.Context, string, string, func(int64, int64)) error {
	return github.ErrRateLimited
}

func TestEnsureBadChecksumKeepsOldBinary(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	m := e.manager()
	if _, err := m.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	e.setFFmpeg("2025-11-30T00:00:00Z", map[string]string{"ffmpeg.exe": "evil", "ffprobe.exe": "evil"}, true)
	e.now = e.now.Add(10 * 24 * time.Hour)
	st, err := m.Ensure(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if readBin(t, e, "ffmpeg.exe") != "ffmpeg" {
		t.Error("old ffmpeg replaced despite bad checksum")
	}
	if len(st.Warnings) == 0 || !strings.Contains(st.Warnings[0], "checksum") {
		t.Errorf("warnings = %v", st.Warnings)
	}
}

func TestEnsureBadChecksumFreshInstallFails(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	e.setFFmpeg("2025-10-30T00:00:00Z", map[string]string{"ffmpeg.exe": "x", "ffprobe.exe": "y"}, true)
	if _, err := e.manager().Ensure(context.Background(), nil); !errors.Is(err, ErrMissingTool) {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.paths.Bin, "ffmpeg.exe")); err == nil {
		t.Error("ffmpeg installed despite bad checksum")
	}
}

func TestEnsureYtdlpRollbackOnWrongVersion(t *testing.T) {
	e := newEnv(t)
	e.seedAll()
	m := e.manager()
	if _, err := m.Ensure(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	e.setYtdlp("2025.11.1", "ytdlp-broken")
	e.versions["yt-dlp.exe"] = "2025.10.22" // new binary reports a wrong version
	st, err := m.Ensure(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if readBin(t, e, "yt-dlp.exe") != "ytdlp-bin" || st.Versions["yt-dlp"] != "2025.10.22" || len(st.Warnings) == 0 {
		t.Errorf("rollback failed: %+v", st)
	}
}

func TestExtractZipSlip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.zip")
	os.WriteFile(archive, zipOf(map[string]string{
		"../../evil.exe":   "evil",
		"x/../ffmpeg.exe":  "ok",
		"nested/deno.exe/": "",
	}), 0o644)
	dst := filepath.Join(dir, "out", "new-ffmpeg.exe")
	os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := extractZip(archive, []string{"ffmpeg.exe"}, []string{dst}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "ok" {
		t.Errorf("content = %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "evil.exe")); err == nil {
		t.Error("zip-slip: file escaped")
	}
	if err := extractZip(archive, []string{"missing.exe"}, []string{dst}); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseChecksumsGetFileHash(t *testing.T) {
	in := "\r\nAlgorithm : SHA256\r\nHash      : A0C3101B\r\nPath      : C:\\x.zip\r\n"
	sums, err := ParseChecksums(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := lookupChecksum(sums, "deno.zip"); !ok || got != "a0c3101b" {
		t.Fatalf("got %q, %v", got, ok)
	}
}

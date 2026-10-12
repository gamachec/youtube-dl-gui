package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/yt-dlp/yt-dlp/releases/latest" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.Write([]byte(`{"tag_name":"2025.01.01","assets":[{"name":"yt-dlp.exe","browser_download_url":"http://x/y","size":42}]}`))
	}))
	defer srv.Close()

	rel, err := (&Client{BaseURL: srv.URL}).LatestRelease(context.Background(), "yt-dlp", "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "2025.01.01" {
		t.Fatalf("tag = %q", rel.TagName)
	}
	a, ok := rel.Asset("yt-dlp.exe")
	if !ok || a.Size != 42 || a.BrowserDownloadURL != "http://x/y" {
		t.Fatalf("asset = %+v ok=%v", a, ok)
	}
	if _, ok := rel.Asset("nope"); ok {
		t.Fatal("unexpected asset")
	}
	if _, ok := rel.AssetMatch(func(n string) bool { return n == "yt-dlp.exe" }); !ok {
		t.Fatal("AssetMatch failed")
	}
}

func TestLatestReleaseErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"404", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }, ErrNotFound},
		{"rate limit 403", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(403)
		}, ErrRateLimited},
		{"rate limit 429", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(429)
		}, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			_, err := (&Client{BaseURL: srv.URL}).LatestRelease(context.Background(), "o", "r")
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("forbidden without rate limit", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
		defer srv.Close()
		_, err := (&Client{BaseURL: srv.URL}).LatestRelease(context.Background(), "o", "r")
		if err == nil || errors.Is(err, ErrRateLimited) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{oops")) }))
		defer srv.Close()
		if _, err := (&Client{BaseURL: srv.URL}).LatestRelease(context.Background(), "o", "r"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("offline", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()
		_, err := (&Client{BaseURL: url}).LatestRelease(context.Background(), "o", "r")
		if !errors.Is(err, ErrOffline) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := (&Client{BaseURL: "http://127.0.0.1:1"}).LatestRelease(ctx, "o", "r")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestDownload(t *testing.T) {
	payload := make([]byte, 200*1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Write(payload)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	var last, lastTotal int64
	err := (&Client{}).Download(context.Background(), srv.URL, dst, func(done, total int64) {
		if done < last {
			t.Errorf("progress went backwards")
		}
		last, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || len(got) != len(payload) {
		t.Fatalf("read: len=%d err=%v", len(got), err)
	}
	if last != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("progress last=%d total=%d", last, lastTotal)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}
}

func TestDownloadFailureKeepsExisting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&Client{}).Download(context.Background(), srv.URL, dst, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "old" {
		t.Fatal("existing file modified")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}
}

func TestDownloadTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte("short"))
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "out.bin")
	if err := (&Client{}).Download(context.Background(), srv.URL, dst, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("dst should not exist")
	}
}

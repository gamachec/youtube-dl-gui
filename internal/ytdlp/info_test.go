package ytdlp

import (
	"os"
	"strings"
	"testing"
)

func TestParseInfo(t *testing.T) {
	data, err := os.ReadFile("testdata/info.json")
	if err != nil {
		t.Fatal(err)
	}
	vi, err := ParseInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if vi.ID != "dQw4w9WgXcQ" || vi.Title != "Rick Astley - Never Gonna Give You Up" || vi.Uploader != "Rick Astley" {
		t.Errorf("métadonnées inattendues: %+v", vi)
	}
	if vi.Duration != 213 || vi.IsLive || vi.Thumbnail == "" || vi.WebpageURL == "" {
		t.Errorf("champs inattendus: %+v", vi)
	}
	if vi.MaxHeight != 2160 {
		t.Errorf("MaxHeight = %d, want 2160 (vignettes exclues)", vi.MaxHeight)
	}
	if !vi.HasAudio {
		t.Error("HasAudio = false")
	}
}

func TestParseInfoInvalid(t *testing.T) {
	if _, err := ParseInfo([]byte("pas du json")); err == nil {
		t.Error("erreur attendue")
	}
}

func TestMapError(t *testing.T) {
	tests := []struct{ stderr, contains string }{
		{"ERROR: [youtube] abc: Video unavailable", "indisponible"},
		{"ERROR: [youtube] abc: Private video. Sign in if you've been granted access", "privée"},
		{"ERROR: [youtube] abc: Sign in to confirm your age. This video may be inappropriate", "âge"},
		{"ERROR: [youtube] abc: This live event will begin in 2 hours.", "pas encore commencé"},
		{"ERROR: Unable to download webpage: <urlopen error [Errno -3]>", "réseau"},
		{"WARNING: x\nERROR: quelque chose d'inconnu", "quelque chose d'inconnu"},
		{"", "sans message"},
	}
	for _, tc := range tests {
		if got := MapError(tc.stderr); !strings.Contains(got, tc.contains) {
			t.Errorf("MapError(%q) = %q, want contains %q", tc.stderr, got, tc.contains)
		}
	}
}

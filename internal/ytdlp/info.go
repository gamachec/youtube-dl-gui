package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cgamache/youtube-dl-gui/internal/procutil"
)

// infoTimeout bounds a metadata lookup.
const infoTimeout = 30 * time.Second

// ErrLive is returned for live streams, unsupported in the MVP.
var ErrLive = errors.New("les lives en cours ne sont pas pris en charge")

// Runner holds the paths of the external tools.
type Runner struct {
	YtDlp     string
	FFmpegDir string
	Deno      string
}

// VideoInfo is the subset of yt-dlp metadata shown in the UI.
type VideoInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Thumbnail  string  `json:"thumbnail"`
	WebpageURL string  `json:"webpageUrl"`
	Duration   float64 `json:"duration"`
	IsLive     bool    `json:"isLive"`
	MaxHeight  int     `json:"maxHeight"`
	HasAudio   bool    `json:"hasAudio"`
}

type rawInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Channel    string  `json:"channel"`
	Thumbnail  string  `json:"thumbnail"`
	WebpageURL string  `json:"webpage_url"`
	Duration   float64 `json:"duration"`
	IsLive     bool    `json:"is_live"`
	Formats    []struct {
		Height int    `json:"height"`
		VCodec string `json:"vcodec"`
		ACodec string `json:"acodec"`
	} `json:"formats"`
}

// ParseInfo decodes the JSON printed by `yt-dlp -J`.
func ParseInfo(data []byte) (VideoInfo, error) {
	var r rawInfo
	if err := json.Unmarshal(data, &r); err != nil {
		return VideoInfo{}, fmt.Errorf("réponse yt-dlp illisible: %w", err)
	}
	vi := VideoInfo{
		ID: r.ID, Title: r.Title, Uploader: r.Uploader, Thumbnail: r.Thumbnail,
		WebpageURL: r.WebpageURL, Duration: r.Duration, IsLive: r.IsLive,
	}
	if vi.Uploader == "" {
		vi.Uploader = r.Channel
	}
	for _, f := range r.Formats {
		if f.VCodec != "" && f.VCodec != "none" && f.Height > vi.MaxHeight {
			vi.MaxHeight = f.Height
		}
		if f.ACodec != "" && f.ACodec != "none" {
			vi.HasAudio = true
		}
	}
	return vi, nil
}

// Info fetches the metadata of a (normalized) video URL.
func (r Runner) Info(ctx context.Context, url string) (VideoInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, infoTimeout)
	defer cancel()

	args := []string{"-J", "--no-playlist", "--no-warnings", "--skip-download"}
	if r.Deno != "" {
		args = append(args, "--js-runtimes", "deno:"+r.Deno)
	}
	args = append(args, url)

	cmd := procutil.Command(ctx, r.YtDlp, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return VideoInfo{}, errors.New("délai dépassé en interrogeant YouTube (30 s)")
		}
		if ctx.Err() != nil {
			return VideoInfo{}, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return VideoInfo{}, errors.New(MapError(stderr.String()))
		}
		return VideoInfo{}, fmt.Errorf("lancement de yt-dlp impossible: %w", err)
	}
	vi, err := ParseInfo(stdout.Bytes())
	if err != nil {
		return VideoInfo{}, err
	}
	if vi.IsLive {
		return VideoInfo{}, ErrLive
	}
	return vi, nil
}

// MapError turns yt-dlp stderr output into a readable French message.
func MapError(stderr string) string {
	s := strings.ToLower(stderr)
	rules := []struct{ needle, msg string }{
		{"no space left on device", "Espace disque insuffisant dans le dossier de sortie."},
		{"not enough space on the disk", "Espace disque insuffisant dans le dossier de sortie."},
		{"private video", "Cette vidéo est privée."},
		{"sign in to confirm your age", "Cette vidéo est soumise à une restriction d'âge (connexion requise)."},
		{"sign in to confirm you", "YouTube demande une vérification (connexion requise)."},
		{"this live event will begin", "Ce live n'a pas encore commencé."},
		{"video unavailable", "Cette vidéo est indisponible."},
		{"not available in your country", "Cette vidéo n'est pas disponible dans votre pays."},
		{"unable to download webpage", "Erreur réseau : impossible de joindre YouTube."},
		{"getaddrinfo failed", "Erreur réseau : impossible de joindre YouTube."},
		{"timed out", "Erreur réseau : délai dépassé."},
		{"temporary failure in name resolution", "Erreur réseau : impossible de joindre YouTube."},
	}
	for _, r := range rules {
		if strings.Contains(s, r.needle) {
			return r.msg
		}
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return "yt-dlp a échoué sans message."
	}
	return "Erreur yt-dlp : " + last
}

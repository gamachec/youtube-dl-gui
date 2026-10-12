// Package ytdlp validates YouTube URLs and drives the yt-dlp binary.
package ytdlp

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	// ErrNotYouTube is returned when the URL does not point to a YouTube host.
	ErrNotYouTube = errors.New("ce lien n'est pas une adresse YouTube")
	// ErrNoVideoID is returned when no valid video ID can be extracted.
	ErrNoVideoID = errors.New("aucun identifiant de vidéo valide dans ce lien")
)

var videoIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

var youtubeHosts = map[string]bool{
	"youtube.com":              true,
	"www.youtube.com":          true,
	"m.youtube.com":            true,
	"music.youtube.com":        true,
	"youtu.be":                 true,
	"youtube-nocookie.com":     true,
	"www.youtube-nocookie.com": true,
}

// NormalizeURL validates raw locally (no network) and returns the canonical
// https://www.youtube.com/watch?v=ID form, dropping list, time and other params.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrNotYouTube
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrNotYouTube
	}
	host := strings.ToLower(u.Hostname())
	if !youtubeHosts[host] {
		return "", ErrNotYouTube
	}

	var id string
	if host == "youtu.be" {
		id = firstSegment(u.Path)
	} else {
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		default:
			for _, prefix := range []string{"/shorts/", "/live/", "/embed/"} {
				if rest, ok := strings.CutPrefix(u.Path, prefix); ok {
					id = firstSegment(rest)
					break
				}
			}
		}
	}
	if !videoIDRe.MatchString(id) {
		return "", ErrNoVideoID
	}
	return "https://www.youtube.com/watch?v=" + id, nil
}

func firstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	return p
}

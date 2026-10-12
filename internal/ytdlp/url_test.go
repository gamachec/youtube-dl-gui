package ytdlp

import (
	"errors"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	const want = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
	tests := []struct {
		name, in, out string
		err           error
	}{
		{"watch", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", want, nil},
		{"sans schéma", "youtube.com/watch?v=dQw4w9WgXcQ", want, nil},
		{"espaces", "  https://youtu.be/dQw4w9WgXcQ \n", want, nil},
		{"youtu.be", "https://youtu.be/dQw4w9WgXcQ", want, nil},
		{"youtu.be avec t", "https://youtu.be/dQw4w9WgXcQ?t=42", want, nil},
		{"playlist ignorée", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PL123&t=10s", want, nil},
		{"mobile", "https://m.youtube.com/watch?v=dQw4w9WgXcQ", want, nil},
		{"music", "https://music.youtube.com/watch?v=dQw4w9WgXcQ", want, nil},
		{"shorts", "https://www.youtube.com/shorts/dQw4w9WgXcQ", want, nil},
		{"live", "https://www.youtube.com/live/dQw4w9WgXcQ?feature=share", want, nil},
		{"embed", "https://www.youtube.com/embed/dQw4w9WgXcQ", want, nil},
		{"nocookie", "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", want, nil},
		{"hôte en majuscules", "HTTPS://WWW.YOUTUBE.COM/watch?v=dQw4w9WgXcQ", want, nil},
		{"id avec - et _", "https://youtu.be/a-b_c-d_e-f", "https://www.youtube.com/watch?v=a-b_c-d_e-f", nil},
		{"http", "http://youtube.com/watch?v=dQw4w9WgXcQ", want, nil},
		{"vide", "", "", ErrNotYouTube},
		{"autre site", "https://vimeo.com/123456789", "", ErrNotYouTube},
		{"faux hôte", "https://youtube.com.evil.com/watch?v=dQw4w9WgXcQ", "", ErrNotYouTube},
		{"schéma ftp", "ftp://youtube.com/watch?v=dQw4w9WgXcQ", "", ErrNotYouTube},
		{"texte libre", "pas une url", "", ErrNotYouTube},
		{"sans v", "https://www.youtube.com/watch", "", ErrNoVideoID},
		{"id trop court", "https://www.youtube.com/watch?v=abc", "", ErrNoVideoID},
		{"id trop long", "https://youtu.be/dQw4w9WgXcQextra", "", ErrNoVideoID},
		{"id invalide", "https://youtu.be/dQw4w9WgX!Q", "", ErrNoVideoID},
		{"page d'accueil", "https://www.youtube.com/", "", ErrNoVideoID},
		{"chaîne", "https://www.youtube.com/@rickastley", "", ErrNoVideoID},
		{"shorts vide", "https://www.youtube.com/shorts/", "", ErrNoVideoID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeURL(tc.in)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if got != tc.out {
				t.Errorf("got %q, want %q", got, tc.out)
			}
		})
	}
}

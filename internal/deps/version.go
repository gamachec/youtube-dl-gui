package deps

import (
	"strconv"
	"strings"
	"time"
)

// CompareYtdlp compares two yt-dlp versions ("2025.10.22", "2025.10.22.1").
// Segments are compared numerically, missing segments count as 0.
// It returns -1, 0 or +1.
func CompareYtdlp(a, b string) int {
	return compareDotted(a, b)
}

// compareSemver compares "vX.Y.Z" style versions, ignoring the leading "v"
// and any pre-release/build suffix.
func compareSemver(a, b string) int {
	return compareDotted(trimSemver(a), trimSemver(b))
}

func trimSemver(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

func compareDotted(a, b string) int {
	as := strings.Split(strings.TrimSpace(a), ".")
	bs := strings.Split(strings.TrimSpace(b), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

// newerTimestamp reports whether latest (RFC 3339) is after installed.
// If either does not parse, any difference counts as newer.
func newerTimestamp(installed, latest string) bool {
	ti, err1 := time.Parse(time.RFC3339, installed)
	tl, err2 := time.Parse(time.RFC3339, latest)
	if err1 != nil || err2 != nil {
		return installed != latest
	}
	return tl.After(ti)
}

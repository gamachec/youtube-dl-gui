package deps

import (
	"time"

	"github.com/cgamache/youtube-dl-gui/internal/github"
)

// Tool describes an external binary managed by the application.
type Tool struct {
	Name          string // "yt-dlp", "ffmpeg", "deno"
	Owner, Repo   string
	AssetName     string   // release asset to download
	ChecksumAsset string   // checksums asset ("" = none); a missing asset only yields a warning
	Files         []string // files installed in bin/ (extracted from the zip, or the asset itself)
	CheckEvery    time.Duration

	// Version returns the identifier of a release used for comparisons.
	Version func(github.Release) string
	// Newer reports whether latest is more recent than installed.
	Newer func(installed, latest string) bool
	// VerifyBinary runs "<Files[0]> --version" after install and expects
	// the release version.
	VerifyBinary bool
}

const week = 7 * 24 * time.Hour

// DefaultTools returns yt-dlp, ffmpeg and deno, in installation order.
func DefaultTools() []Tool {
	return []Tool{
		{
			Name: "yt-dlp", Owner: "yt-dlp", Repo: "yt-dlp",
			AssetName: "yt-dlp.exe", ChecksumAsset: "SHA2-256SUMS",
			Files:        []string{"yt-dlp.exe"},
			Version:      func(r github.Release) string { return r.TagName },
			Newer:        func(i, l string) bool { return CompareYtdlp(i, l) < 0 },
			VerifyBinary: true,
		},
		{
			Name: "ffmpeg", Owner: "yt-dlp", Repo: "FFmpeg-Builds",
			AssetName: "ffmpeg-master-latest-win64-gpl.zip", ChecksumAsset: "checksums.sha256",
			Files:      []string{"ffmpeg.exe", "ffprobe.exe"},
			CheckEvery: week,
			// The tag is always "latest": use the publication date instead.
			Version: func(r github.Release) string { return r.PublishedAt },
			Newer:   newerTimestamp,
		},
		{
			Name: "deno", Owner: "denoland", Repo: "deno",
			AssetName: "deno-x86_64-pc-windows-msvc.zip", ChecksumAsset: "deno-x86_64-pc-windows-msvc.zip.sha256sum",
			Files:      []string{"deno.exe"},
			CheckEvery: week,
			Version:    func(r github.Release) string { return r.TagName },
			Newer:      func(i, l string) bool { return compareSemver(i, l) < 0 },
		},
	}
}

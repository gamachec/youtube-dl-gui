package ytdlp

import "strconv"

// Mode selects what is downloaded.
type Mode string

const (
	ModeVideoAudio Mode = "video_audio"
	ModeVideoOnly  Mode = "video_only"
	ModeAudioOnly  Mode = "audio_only"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	return m == ModeVideoAudio || m == ModeVideoOnly || m == ModeAudioOnly
}

// Request describes one download.
type Request struct {
	URL       string
	OutputDir string
	Mode      Mode
}

// BinPaths locates the external tools yt-dlp needs.
type BinPaths struct {
	FFmpegDir string
	Deno      string
}

const (
	outputTemplate = "%(title)s [%(id)s].%(ext)s"
	progressPrefix = "PROG"
	filePrefix     = "FILE"
	progressTmpl   = "download:" + progressPrefix + "|%(progress.status)s|%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s|%(info.format_id)s"
	mergeAudioArgs = "Merger+ffmpeg_o:-c:v copy -c:a aac -b:a 192k"
)

// BuildArgs builds the yt-dlp command line for a request (pure function).
func BuildArgs(r Request, bin BinPaths) []string {
	args := []string{
		"--no-playlist", "--newline", "--no-colors", "--no-warnings",
		"--ffmpeg-location", bin.FFmpegDir,
	}
	if bin.Deno != "" {
		args = append(args, "--js-runtimes", "deno:"+bin.Deno)
	}
	args = append(args,
		"-P", r.OutputDir,
		"-o", outputTemplate,
		"--windows-filenames",
		"--trim-filenames", strconv.Itoa(180),
		"--no-mtime",
		"--progress-template", progressTmpl,
		"--print", "after_move:"+filePrefix+"|%(filepath)s",
		// --print implies --quiet, which would hide the stage lines we parse.
		"--no-quiet", "--progress",
	)
	switch r.Mode {
	case ModeVideoOnly:
		args = append(args, "-f", "bv*", "--remux-video", "mp4")
	case ModeAudioOnly:
		args = append(args, "-f", "ba/b", "-x", "--audio-format", "mp3", "--audio-quality", "0")
	default:
		args = append(args, "-f", "bv*+ba/b", "--merge-output-format", "mp4", "--postprocessor-args", mergeAudioArgs)
	}
	return append(args, "--", r.URL)
}

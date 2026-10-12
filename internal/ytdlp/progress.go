package ytdlp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Event is a progress update sent to the UI.
type Event struct {
	Phase      string  `json:"phase"`
	Percent    float64 `json:"percent"`
	Speed      string  `json:"speed"`
	ETA        string  `json:"eta"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
}

const (
	phaseVideo   = "Téléchargement vidéo"
	phaseAudio   = "Téléchargement audio"
	phaseGeneric = "Téléchargement"
	phaseMerge   = "Fusion"
	phaseMP3     = "Conversion MP3"
	phaseMP4     = "Conversion MP4"
)

// Overall percentage budget: download phases then post-processing.
const (
	pctVideoEnd = 85.0
	pctAudioEnd = 95.0
	pctPostBase = 95.0
)

var idInName = regexp.MustCompile(`\[([A-Za-z0-9_-]{11})\]`)

// parser turns yt-dlp output lines into events. It is a pure state machine.
type parser struct {
	mode    Mode
	files   int // downloads started so far
	phase   string
	maxPct  float64
	videoID string
	file    string
}

func newParser(mode Mode) *parser { return &parser{mode: mode} }

// feed consumes one line and returns an event when the line changes
// something visible.
func (p *parser) feed(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, progressPrefix+"|"):
		return p.progress(line)
	case strings.HasPrefix(line, filePrefix+"|"):
		p.file = strings.TrimPrefix(line, filePrefix+"|")
		if m := idInName.FindStringSubmatch(p.file); m != nil {
			p.videoID = m[1]
		}
		return Event{}, false
	case strings.HasPrefix(line, "[download] Destination:"):
		dest := strings.TrimSpace(strings.TrimPrefix(line, "[download] Destination:"))
		if m := idInName.FindStringSubmatch(dest); m != nil {
			p.videoID = m[1]
		}
		p.files++
		p.phase = p.downloadPhase()
		return p.event(Event{Phase: p.phase}, p.base()), true
	case strings.HasPrefix(line, "[download]") && strings.HasSuffix(line, "has already been downloaded"):
		p.files++
		return p.event(Event{Phase: p.downloadPhase()}, p.base()+p.width()), true
	case strings.HasPrefix(line, "[Merger]"):
		return p.post(phaseMerge)
	case strings.HasPrefix(line, "[ExtractAudio]"):
		return p.post(phaseMP3)
	case strings.HasPrefix(line, "[VideoRemuxer]"):
		return p.post(phaseMP4)
	}
	return Event{}, false
}

func (p *parser) downloadPhase() string {
	switch p.mode {
	case ModeVideoOnly:
		return phaseVideo
	case ModeAudioOnly:
		return phaseAudio
	}
	switch p.files {
	case 1:
		return phaseVideo
	case 2:
		return phaseAudio
	}
	return phaseGeneric
}

// base and width give the overall-percent window of the current download.
func (p *parser) base() float64 {
	if p.mode == ModeVideoAudio {
		switch {
		case p.files <= 1:
			return 0
		case p.files == 2:
			return pctVideoEnd
		}
		return pctAudioEnd
	}
	return 0
}

func (p *parser) width() float64 {
	if p.mode == ModeVideoAudio {
		switch {
		case p.files <= 1:
			return pctVideoEnd
		case p.files == 2:
			return pctAudioEnd - pctVideoEnd
		}
		return 0
	}
	return pctAudioEnd
}

func (p *parser) post(phase string) (Event, bool) {
	p.phase = phase
	return p.event(Event{Phase: phase}, pctPostBase), true
}

// event fills the phase and a monotone overall percentage.
func (p *parser) event(e Event, pct float64) Event {
	if pct > p.maxPct {
		p.maxPct = pct
	}
	e.Percent = p.maxPct
	if e.Phase == "" {
		e.Phase = p.phase
	}
	return e
}

func (p *parser) progress(line string) (Event, bool) {
	f := strings.Split(line, "|")
	if len(f) < 7 {
		return Event{}, false
	}
	downloaded := int64(parseNum(f[2]))
	total := int64(parseNum(f[3]))
	if total == 0 {
		total = int64(parseNum(f[4]))
	}
	if p.files == 0 { // Destination line missed: still count the file
		p.files = 1
		p.phase = p.downloadPhase()
	}
	filePct := 0.0
	if total > 0 {
		filePct = float64(downloaded) / float64(total)
		if filePct > 1 {
			filePct = 1
		}
	}
	e := Event{
		Downloaded: downloaded,
		Total:      total,
		Speed:      formatSpeed(parseNum(f[5])),
		ETA:        formatETA(parseNum(f[6])),
	}
	return p.event(e, p.base()+filePct*p.width()), true
}

// parseNum parses a numeric field; "NA", "None" and garbage give 0.
func parseNum(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func formatSpeed(bps float64) string {
	if bps <= 0 {
		return ""
	}
	units := []string{"o/s", "Kio/s", "Mio/s", "Gio/s"}
	i := 0
	for bps >= 1024 && i < len(units)-1 {
		bps /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", bps, units[i])
}

func formatETA(sec float64) string {
	if sec <= 0 {
		return ""
	}
	s := int(sec)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

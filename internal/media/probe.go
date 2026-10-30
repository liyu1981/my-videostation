// Package media wraps ffprobe to extract technical metadata from video files.
package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNoProber is returned when no ffprobe binary could be found.
var ErrNoProber = errors.New("ffprobe not found")

// Prober shells out to ffprobe.
type Prober struct {
	Path string
}

// AudioStream describes one audio track.
type AudioStream struct {
	Index      int    `json:"index"`
	Codec      string `json:"codec"`
	Channels   int    `json:"channels"`
	Layout     string `json:"layout,omitempty"`
	SampleRate int    `json:"sampleRate,omitempty"`
	Bitrate    int64  `json:"bitrate,omitempty"`
	Language   string `json:"language,omitempty"`
	Title      string `json:"title,omitempty"`
	Default    bool   `json:"default,omitempty"`
}

// SubtitleStream describes one subtitle track.
type SubtitleStream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
	External bool   `json:"external,omitempty"`
}

// Technical is the subset of ffprobe output we persist.
type Technical struct {
	Container  string
	DurationMS int64
	Bitrate    int64
	Width      int
	Height     int
	VideoCodec string
	HDR        string
	Audio      []AudioStream
	Subtitles  []SubtitleStream
}

// Detect locates ffprobe using (in order): explicit path, $MVS_FFPROBE, PATH,
// ./.tools/ffprobe, /usr/bin/ffprobe. Returns ErrNoProber when missing.
func Detect(explicit string) (*Prober, error) {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if v := os.Getenv("MVS_FFPROBE"); v != "" {
		candidates = append(candidates, v)
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, ".tools/ffprobe", "bin/ffprobe", "/usr/bin/ffprobe", "/usr/local/bin/ffprobe")

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return &Prober{Path: c}, nil
		}
	}
	return nil, ErrNoProber
}

type ffprobeOutput struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index         int               `json:"index"`
		CodecName     string            `json:"codec_name"`
		CodecType     string            `json:"codec_type"`
		Width         int               `json:"width"`
		Height        int               `json:"height"`
		Channels      int               `json:"channels"`
		ChannelLayout string            `json:"channel_layout"`
		SampleRate    string            `json:"sample_rate"`
		BitRate       string            `json:"bit_rate"`
		Profile       string            `json:"profile"`
		PixFmt        string            `json:"pix_fmt"`
		ColorTransfer string            `json:"color_transfer"`
		Tags          map[string]string `json:"tags"`
		Disposition   map[string]int    `json:"disposition"`
		SideDataList  []struct {
			SideDataType string `json:"side_data_type"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

// Probe runs ffprobe and maps its output to Technical.
func (p *Prober) Probe(ctx context.Context, path string) (*Technical, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.Path,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_entries", "stream=index,codec_name,codec_type,width,height,channels,channel_layout,sample_rate,bit_rate,profile,pix_fmt,color_transfer:stream_tags=language,title:stream_disposition=default,forced:format=format_name,duration,size,bit_rate",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("ffprobe %s: %s", filepath.Base(path), strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}

	var raw ffprobeOutput
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	t := &Technical{
		Container: raw.Format.FormatName,
		Bitrate:   parseInt(raw.Format.BitRate),
		DurationMS: func() int64 {
			if f, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil {
				return int64(f * 1000)
			}
			return 0
		}(),
		Audio:     []AudioStream{},
		Subtitles: []SubtitleStream{},
	}

	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if t.Width == 0 && s.Width > 0 {
				t.Width, t.Height = s.Width, s.Height
				t.VideoCodec = s.CodecName
				t.HDR = detectHDR(s)
			}
		case "audio":
			t.Audio = append(t.Audio, AudioStream{
				Index:      s.Index,
				Codec:      s.CodecName,
				Channels:   s.Channels,
				Layout:     s.ChannelLayout,
				SampleRate: int(parseInt(s.SampleRate)),
				Bitrate:    parseInt(s.BitRate),
				Language:   s.Tags["language"],
				Title:      s.Tags["title"],
				Default:    s.Disposition["default"] == 1,
			})
		case "subtitle":
			t.Subtitles = append(t.Subtitles, SubtitleStream{
				Index:    s.Index,
				Codec:    s.CodecName,
				Language: s.Tags["language"],
				Title:    s.Tags["title"],
				Forced:   s.Disposition["forced"] == 1,
			})
		}
	}
	return t, nil
}

func detectHDR(s struct {
	Index         int               `json:"index"`
	CodecName     string            `json:"codec_name"`
	CodecType     string            `json:"codec_type"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	SampleRate    string            `json:"sample_rate"`
	BitRate       string            `json:"bit_rate"`
	Profile       string            `json:"profile"`
	PixFmt        string            `json:"pix_fmt"`
	ColorTransfer string            `json:"color_transfer"`
	Tags          map[string]string `json:"tags"`
	Disposition   map[string]int    `json:"disposition"`
	SideDataList  []struct {
		SideDataType string `json:"side_data_type"`
	} `json:"side_data_list"`
}) string {
	for _, sd := range s.SideDataList {
		if strings.Contains(strings.ToLower(sd.SideDataType), "dolby vision") {
			return "Dolby Vision"
		}
	}
	switch strings.ToLower(s.ColorTransfer) {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	if strings.Contains(strings.ToLower(s.Profile), "main 10") {
		return "HDR10"
	}
	return ""
}

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

var artNames = []string{"poster", "folder", "cover", "fanart", "backdrop"}

// FindLocalArt looks for sidecar artwork next to the file and one directory up.
// Returns absolute paths ("" when absent). Media folders are never modified.
func FindLocalArt(videoPath string) (poster, backdrop string) {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	dirs := []string{dir, filepath.Dir(dir)}
	exts := []string{".jpg", ".jpeg", ".png", ".webp"}

	find := func(name string, dirs []string) string {
		for _, d := range dirs {
			for _, ext := range exts {
				p := filepath.Join(d, name+ext)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
		return ""
	}
	poster = find(base, dirs)
	if poster == "" {
		for _, n := range artNames {
			if n == "fanart" || n == "backdrop" {
				continue
			}
			if p := find(n, dirs); p != "" {
				poster = p
				break
			}
		}
	}
	backdrop = find("fanart", dirs)
	if backdrop == "" {
		backdrop = find("backdrop", dirs)
	}
	if backdrop == "" {
		backdrop = poster
	}
	return poster, backdrop
}

// SubtitleFile is a sidecar subtitle discovered next to a video.
type SubtitleFile struct {
	Path     string
	Language string
	Label    string
	Format   string
}

// FindSubtitles lists sidecar subtitles for a video file: <stem>.<ext> or
// <stem>.<lang>.<ext> with srt/ass/ssa/vtt/sub extensions.
func FindSubtitles(videoPath string) []SubtitleFile {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	baseLower := strings.ToLower(base)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	allowed := map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true}

	var out []SubtitleFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !allowed[ext] {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		stemLower := strings.ToLower(stem)
		lang := ""
		switch {
		case stemLower == baseLower:
			// no language marker
		case strings.HasPrefix(stemLower, baseLower+"."):
			lang = strings.TrimPrefix(stemLower, baseLower+".")
			if len(lang) > 20 || strings.ContainsAny(lang, " ./\\") {
				continue
			}
		default:
			continue
		}
		label := "External"
		if lang != "" {
			label = strings.ToUpper(lang)
		}
		out = append(out, SubtitleFile{
			Path:     filepath.Join(dir, name),
			Language: lang,
			Label:    label,
			Format:   strings.TrimPrefix(ext, "."),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// DetectFFmpeg locates the ffmpeg binary (used for embedded subtitle extraction).
func DetectFFmpeg(explicit string) string {
	candidates := []string{}
	if explicit != "" {
		candidates = append(candidates, explicit)
	}
	if v := os.Getenv("MVS_FFMPEG"); v != "" {
		candidates = append(candidates, v)
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, ".tools/ffmpeg", "bin/ffmpeg", "/usr/bin/ffmpeg", "/usr/local/bin/ffmpeg")
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

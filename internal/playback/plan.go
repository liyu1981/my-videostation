package playback

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Mode is how a media file reaches the client.
type Mode string

const (
	// ModeRemux copies both video and audio into HLS fMP4 without re-encoding.
	ModeRemux Mode = "remux"
	// ModeTranscode re-encodes at least one stream.
	ModeTranscode Mode = "transcode"
)

// AudioStream is one audio track as reported by ffprobe (absolute stream index).
type AudioStream struct {
	Index    int
	Codec    string
	Channels int
	Language string
	Title    string
	Default  bool
}

// SubtitleStream is one subtitle track (absolute stream index).
type SubtitleStream struct {
	Index    int
	Codec    string
	Language string
	Title    string
	Forced   bool
}

// Request describes one playback session to start.
type Request struct {
	FileID     int64  // for logging/keys only
	Path       string // absolute media path
	Mtime      int64  // invalidates sessions when the file changes
	Container  string
	VideoCodec string
	Width      int
	Height     int
	HDR        string
	Audio      []AudioStream
	Subtitles  []SubtitleStream

	// AudioIndex selects an audio stream by absolute ffprobe index; -1 = first.
	AudioIndex int
	// BurnSubtitle selects an embedded subtitle stream to burn in; -1 = none.
	BurnSubtitle int
	// BurnSubtitlePath burns an external subtitle file (SRT/ASS/VTT); "" = none.
	BurnSubtitlePath string

	DurationMS int64
	StartMS    int64
	MaxHeight  int    // 0 = source height
	Force      string // "", "remux" or "transcode"

	OutDir string // session directory (created by the manager)
}

// Plan is the resolved ffmpeg invocation for one session.
type Plan struct {
	Mode         Mode
	Args         []string
	StartMS      int64
	DurationMS   int64
	Width        int
	Height       int
	VideoCodec   string
	AudioCodec   string
	AudioIndex   int
	BurnSubtitle int
	Container    string
	Reasons      []string
}

// noUpscale caps the output height at the source height.
func noUpscale(height, max int) int {
	if height <= 0 {
		return max
	}
	if max <= 0 || max > height {
		return height
	}
	// Keep even dimensions for yuv420p.
	if max%2 == 1 {
		max++
	}
	return max
}

// copyableVideo reports whether a video codec can be copied into fMP4 HLS and
// decoded by the browsers we target. HEVC/HDR deliberately re-encode: hls.js
// MSE support for HEVC is still browser-dependent, and the M5 goal is "play
// anything", not "play anything natively when lucky".
func copyableVideo(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "avc", "avc1":
		return true
	}
	return false
}

// copyableAudio reports whether an audio codec can be copied into fMP4 HLS.
func copyableAudio(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "aac", "mp3", "mp2":
		return true
	}
	return false
}

// EncoderProfile is a named x264/NVENC/QSV profile.
type EncoderProfile struct {
	Name  string // libx264, h264_nvenc, h264_qsv
	Label string
	Args  []string
	// Suffix is appended to the video filter chain when set (QSV needs a
	// hardware upload, for example).
	Filter string
}

// Encoders returns the supported software/hardware encoder profiles.
func EncoderProfiles() []EncoderProfile {
	return []EncoderProfile{
		{Name: "libx264", Label: "software (x264)", Args: []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-profile:v", "high", "-pix_fmt", "yuv420p"}},
		{Name: "h264_nvenc", Label: "NVIDIA NVENC", Args: []string{"-c:v", "h264_nvenc", "-preset", "p4", "-b:v", "0", "-cq", "23", "-pix_fmt", "yuv420p"}},
		{Name: "h264_qsv", Label: "Intel Quick Sync", Args: []string{"-c:v", "h264_qsv", "-global_quality", "23", "-pix_fmt", "nv12"}, Filter: "format=nv12"},
	}
}

// options carries the tunables a Plan needs.
type options struct {
	Encoder  string
	HWAccel  string
	Paced    bool // throttle ffmpeg to ~1x so it does not encode the whole file
	Readrate bool // ffmpeg supports -readrate (detected)
}

// forceKeyframes pins keyframes to the 4 s segment boundaries so a transcode
// starts playing (and seeks) without waiting for a long source GOP.
var forceKeyframes = []string{"-force_key_frames", "expr:gte(t,n_forced*4)"}

func segmentArgs(outDir string) []string {
	return []string{
		"-f", "hls",
		"-hls_time", "4",
		"-hls_playlist_type", "event",
		"-hls_list_size", "0",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", filepath.Join(outDir, "seg%05d.m4s"),
	}
}

// BuildPlan resolves remux-vs-transcode, stream selection, scaling and the
// ffmpeg argument list for a session.
func BuildPlan(req Request, o options) (*Plan, error) {
	if req.Path == "" {
		return nil, fmt.Errorf("no media path")
	}
	p := &Plan{
		StartMS:      req.StartMS,
		DurationMS:   maxInt64(0, req.DurationMS-absInt64(req.StartMS)),
		VideoCodec:   strings.ToLower(req.VideoCodec),
		AudioIndex:   0,
		BurnSubtitle: -1,
		Container:    strings.ToLower(req.Container),
	}

	// Audio selection: -1 means "first stream", which ffmpeg addresses as 0:a:0.
	audioAbs := -1
	if len(req.Audio) > 0 {
		audioAbs = req.Audio[0].Index
		p.AudioCodec = strings.ToLower(req.Audio[0].Codec)
		for _, a := range req.Audio {
			if a.Index == req.AudioIndex {
				audioAbs = a.Index
				p.AudioCodec = strings.ToLower(a.Codec)
				break
			}
		}
	}
	p.AudioIndex = audioAbs

	videoCopy := copyableVideo(req.VideoCodec) && req.BurnSubtitle < 0 && req.BurnSubtitlePath == ""
	audioCopy := audioAbs >= 0 && copyableAudio(p.AudioCodec)
	if req.BurnSubtitle >= 0 || req.BurnSubtitlePath != "" {
		p.Reasons = append(p.Reasons, "subtitle burn-in")
	}
	if !videoCopy && req.VideoCodec != "" {
		p.Reasons = append(p.Reasons, fmt.Sprintf("%s video", strings.ToUpper(req.VideoCodec)))
	}
	if audioAbs >= 0 && !audioCopy {
		p.Reasons = append(p.Reasons, fmt.Sprintf("%s audio", strings.ToUpper(p.AudioCodec)))
	}
	if strings.EqualFold(req.Force, string(ModeTranscode)) {
		videoCopy, audioCopy = false, false
		p.Reasons = append(p.Reasons, "transcode requested")
	}
	if strings.EqualFold(req.Force, string(ModeRemux)) {
		videoCopy = copyableVideo(req.VideoCodec) && req.BurnSubtitle < 0 && req.BurnSubtitlePath == ""
		audioCopy = audioAbs >= 0 && copyableAudio(p.AudioCodec)
	}

	height := noUpscale(req.Height, req.MaxHeight)
	// A stream copy cannot be scaled, so asking for a smaller rendition means
	// re-encoding even when the codec itself would have been copyable.
	if videoCopy && req.MaxHeight > 0 && req.Height > 0 && req.MaxHeight < req.Height {
		videoCopy = false
		p.Reasons = append(p.Reasons, fmt.Sprintf("downscale to %dp", height))
	}
	if videoCopy && audioCopy {
		p.Mode = ModeRemux
	} else {
		p.Mode = ModeTranscode
	}
	if videoCopy && req.Height > 0 {
		height = req.Height
	}
	p.Width = req.Width
	p.Height = height
	if req.Height > 0 && height != req.Height && req.Width > 0 {
		p.Width = int(float64(req.Width) * float64(height) / float64(req.Height))
		if p.Width%2 == 1 {
			p.Width++
		}
	}

	var enc EncoderProfile
	if !videoCopy {
		enc = encoderByName(o.Encoder)
	}
	var filters []string
	if !videoCopy && req.Height > 0 && height > 0 && height != req.Height {
		filters = append(filters, fmt.Sprintf("scale=-2:%d", height))
	}
	if req.HDR != "" && !videoCopy {
		p.Reasons = append(p.Reasons, "HDR downconverted to 8-bit SDR")
	}
	if req.BurnSubtitlePath != "" {
		filters = append(filters, "subtitles="+escapeFilterValue(req.BurnSubtitlePath))
	}
	if !videoCopy {
		filters = append(filters, "setsar=1")
		if enc.Filter != "" {
			filters = append(filters, enc.Filter)
		}
	}

	args := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error", "-nostats",
		"-progress", "pipe:1",
	}
	if o.Paced {
		if o.Readrate {
			args = append(args, "-readrate", "1.0", "-readrate_initial_burst", "12")
		} else {
			args = append(args, "-re")
		}
	}
	if o.HWAccel != "" {
		args = append(args, "-hwaccel", o.HWAccel)
	}
	if req.StartMS >= 3000 {
		args = append(args, "-ss", secondsString(req.StartMS))
	}
	args = append(args, "-i", req.Path)
	args = append(args, "-max_muxing_queue_size", "1024")

	if req.BurnSubtitle >= 0 {
		// Bitmap subtitles (PGS/VobSub) are composited with overlay. The whole
		// filter chain (scale/SAR/encoder format + overlay) lives in one graph,
		// because -vf and -filter_complex cannot be mixed for the same stream.
		p.BurnSubtitle = req.BurnSubtitle
		graph := "[0:v:0]"
		if len(filters) > 0 {
			graph += strings.Join(filters, ",") + "[pre];[pre]"
		}
		graph += fmt.Sprintf("[0:%d]overlay[vout]", req.BurnSubtitle)
		args = append(args, "-filter_complex", graph, "-map", "[vout]")
	} else {
		args = append(args, "-map", "0:v:0")
	}
	if audioAbs >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(audioAbs))
	}
	switch {
	case videoCopy:
		args = append(args, "-c:v", "copy")
	case req.BurnSubtitle >= 0:
		args = append(args, enc.Args...)
		args = append(args, forceKeyframes...)
	default:
		args = append(args, enc.Args...)
		args = append(args, forceKeyframes...)
		if len(filters) > 0 {
			args = append(args, "-vf", strings.Join(filters, ","))
		}
	}
	if audioAbs >= 0 {
		if audioCopy {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "160k")
			if channels(req.Audio, audioAbs) > 2 {
				args = append(args, "-ac", "2")
			}
		}
	}
	args = append(args, segmentArgs(req.OutDir)...)
	args = append(args, filepath.Join(req.OutDir, "index.m3u8"))
	p.Args = args
	return p, nil
}

func encoderByName(name string) EncoderProfile {
	for _, e := range EncoderProfiles() {
		if e.Name == name {
			return e
		}
	}
	return EncoderProfiles()[0]
}

func channels(tracks []AudioStream, abs int) int {
	for _, a := range tracks {
		if a.Index == abs {
			return a.Channels
		}
	}
	return 0
}

func secondsString(ms int64) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', 3, 64)
}

// escapeFilterValue quotes a filename for use inside an ffmpeg filtergraph.
func escapeFilterValue(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`:`, `\:`,
		`,`, `\,`,
		`;`, `\;`,
		`[`, `\[`,
		`]`, `\]`,
	)
	return r.Replace(s)
}

// QualityOption is one entry of the client-facing quality ladder.
type QualityOption struct {
	Label  string `json:"label"`
	Height int    `json:"height"`
}

// QualityLadder returns the resolutions a source can be transcoded to.
func QualityLadder(sourceHeight int) []QualityOption {
	out := []QualityOption{{Label: "Auto", Height: 0}}
	if sourceHeight <= 0 {
		return out
	}
	for _, h := range []int{2160, 1080, 720, 480, 360, 240} {
		if h < sourceHeight {
			out = append(out, QualityOption{Label: fmt.Sprintf("%dp", h), Height: h})
		}
	}
	return out
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// directContainers are the containers a stock browser <video> element handles.
var directContainers = map[string]bool{
	"mp4": true, "m4v": true, "mov": true, "webm": true, "ogv": true, "ogg": true,
}

// directVideo are the video codecs browsers decode without help.
var directVideo = map[string]bool{
	"h264": true, "avc": true, "avc1": true,
	"vp8": true, "vp9": true, "av1": true,
}

// directAudio are the audio codecs browsers decode without help.
var directAudio = map[string]bool{
	"aac": true, "mp3": true, "opus": true, "vorbis": true, "flac": true,
}

// ContainerFromPath canonicalises the container. ffprobe reports both Matroska
// and WebM as "matroska,webm", so the file extension is the only reliable
// distinction for the direct-play decision.
func ContainerFromPath(path, probed string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv":
		return "matroska"
	case ".webm":
		return "webm"
	case ".mp4", ".m4v":
		return "mp4"
	case ".mov":
		return "mov"
	case ".ogv", ".ogg":
		return "ogg"
	case ".ts", ".m2ts", ".mts":
		return "mpegts"
	case ".avi":
		return "avi"
	case ".mpg", ".mpeg":
		return "mpeg"
	}
	return strings.ToLower(strings.TrimSpace(probed))
}

// anyContainerDirect matches ffprobe's comma-separated container names
// (e.g. "mov,mp4,m4a,3gp,3g2,mj2" for an MP4 file).
func anyContainerDirect(names string) bool {
	for _, part := range strings.Split(names, ",") {
		if directContainers[strings.TrimSpace(part)] {
			return true
		}
	}
	return false
}

func displayContainer(names string) string {
	parts := strings.Split(names, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts[0]
}

// DirectPlayDecision explains whether the browser can play a file as-is.
type DirectPlayDecision struct {
	OK      bool     `json:"ok"`
	Reasons []string `json:"reasons,omitempty"`
}

// EvaluateDirectPlay decides whether a file can be streamed to a stock
// browser <video> element without remuxing or transcoding.
func EvaluateDirectPlay(container, videoCodec string, audioCodecs []string) DirectPlayDecision {
	d := DirectPlayDecision{OK: true}
	c := strings.ToLower(strings.TrimSpace(container))
	if c != "" && !anyContainerDirect(c) {
		d.OK = false
		d.Reasons = append(d.Reasons, displayContainer(c)+" container")
	}
	v := strings.ToLower(strings.TrimSpace(videoCodec))
	if v != "" && !directVideo[v] {
		d.OK = false
		d.Reasons = append(d.Reasons, strings.ToUpper(v)+" video")
	}
	for _, a := range audioCodecs {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !directAudio[a] {
			d.OK = false
			d.Reasons = append(d.Reasons, strings.ToUpper(a)+" audio")
			break
		}
	}
	return d
}

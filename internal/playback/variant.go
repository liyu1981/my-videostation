package playback

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// VariantSpec identifies a background-converted file (ADR 0006). Every field
// that changes the output bytes is part of the key.
type VariantSpec struct {
	FileID           int64
	Mtime            int64
	Size             int64
	MaxHeight        int
	AudioIndex       int
	BurnSubtitle     int
	BurnSubtitlePath string
	Encoder          string
}

// Key is the stable cache key for one variant.
func (s VariantSpec) Key() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%d|%d|%d|%d|%d|%d|%s|%s",
		s.FileID, s.Mtime, s.Size, s.MaxHeight, s.AudioIndex,
		s.BurnSubtitle, s.BurnSubtitlePath, s.Encoder)))
	return hex.EncodeToString(sum[:])[:32]
}

// VariantJob is the durable job payload for one background conversion.
type VariantJob struct {
	VariantID        int64  `json:"variantId"`
	FileID           int64  `json:"fileId"`
	ItemID           int64  `json:"itemId"`
	MaxHeight        int    `json:"maxHeight"`
	AudioIndex       int    `json:"audioIndex"`
	BurnSubtitle     int    `json:"burnSubtitle"`
	BurnSubtitlePath string `json:"burnSubtitlePath,omitempty"`
	OutPath          string `json:"outPath"`
	VariantKey       string `json:"variantKey"`
}

// VariantPlan is the resolved ffmpeg invocation that writes one MP4 file.
type VariantPlan struct {
	Key          string
	Args         []string
	OutPath      string
	DurationMS   int64
	Width        int
	Height       int
	VideoCodec   string
	AudioCodec   string
	AudioIndex   int
	BurnSubtitle int
	CopyVideo    bool
	CopyAudio    bool
	Reasons      []string
}

// BuildVariantPlanFor is the exported entry point used by the job runner.
func BuildVariantPlanFor(req Request, spec VariantSpec, encoder, hwaccel, outPath string) (*VariantPlan, error) {
	return BuildVariantPlan(req, spec, options{Encoder: encoder, HWAccel: hwaccel}, outPath)
}

// BuildVariantPlan resolves a full-file conversion into a progressive MP4.
// Unlike the HLS session plan it has no seek, no segmenting and no pacing: the
// job runs as fast as the worker allows and writes one file.
func BuildVariantPlan(req Request, spec VariantSpec, o options, outPath string) (*VariantPlan, error) {
	// The variant always covers the whole file; a client seeking inside it is
	// served by HTTP range requests, not by restarting ffmpeg.
	req.StartMS = 0
	rs, err := resolveStreams(req, o)
	if err != nil {
		return nil, err
	}
	p := &VariantPlan{
		Key:          spec.Key(),
		OutPath:      outPath,
		DurationMS:   req.DurationMS,
		Width:        rs.width,
		Height:       rs.height,
		VideoCodec:   rs.videoCodec,
		AudioCodec:   rs.audioCodec,
		AudioIndex:   rs.audioAbs,
		BurnSubtitle: rs.burnSubtitle,
		CopyVideo:    rs.videoCopy,
		CopyAudio:    rs.audioCopy,
		Reasons:      rs.reasons,
	}

	args := []string{
		"-hide_banner", "-nostdin", "-loglevel", "error", "-nostats",
		"-progress", "pipe:1", "-y",
	}
	if o.HWAccel != "" {
		args = append(args, "-hwaccel", o.HWAccel)
	}
	args = append(args, "-i", req.Path)
	args = append(args, "-max_muxing_queue_size", "1024")

	if req.BurnSubtitle >= 0 {
		graph := "[0:v:0]"
		if len(rs.filters) > 0 {
			graph += strings.Join(rs.filters, ",") + "[pre];[pre]"
		}
		graph += fmt.Sprintf("[0:%d]overlay[vout]", req.BurnSubtitle)
		args = append(args, "-filter_complex", graph, "-map", "[vout]")
	} else {
		args = append(args, "-map", "0:v:0")
	}
	if rs.audioAbs >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(rs.audioAbs))
	} else {
		args = append(args, "-an")
	}

	switch {
	case rs.videoCopy:
		args = append(args, "-c:v", "copy")
	case req.BurnSubtitle >= 0:
		args = append(args, rs.enc.Args...)
	default:
		args = append(args, rs.enc.Args...)
		if len(rs.filters) > 0 {
			args = append(args, "-vf", strings.Join(rs.filters, ","))
		}
	}
	if rs.audioAbs >= 0 {
		if rs.audioCopy {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "160k")
			if channels(req.Audio, rs.audioAbs) > 2 {
				args = append(args, "-ac", "2")
			}
		}
	}
	args = append(args, "-movflags", "+faststart", "-f", "mp4", outPath)
	p.Args = args
	return p, nil
}

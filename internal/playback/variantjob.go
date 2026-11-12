package playback

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/liyu1981/myvideostation/internal/store"
)

// RunVariantJob executes one background conversion and updates the variant row.
// It is shared by the server's job handler and the tests.
func RunVariantJob(ctx context.Context, st *store.Store, ffmpeg, encoder, hwaccel string, job *store.Job, payload VariantJob, report func(float64, string)) error {
	if ffmpeg == "" {
		return fmt.Errorf("ffmpeg is not available")
	}
	if payload.VariantID == 0 || payload.FileID == 0 || payload.OutPath == "" {
		return fmt.Errorf("missing transcode payload fields")
	}
	file, err := st.FileByID(ctx, payload.FileID)
	if err != nil {
		return err
	}
	item, err := st.GetItem(ctx, file.ItemID)
	if err != nil {
		return err
	}
	spec := VariantSpec{
		FileID:           file.ID,
		Mtime:            file.Mtime,
		Size:             file.Size,
		MaxHeight:        payload.MaxHeight,
		AudioIndex:       payload.AudioIndex,
		BurnSubtitle:     payload.BurnSubtitle,
		BurnSubtitlePath: payload.BurnSubtitlePath,
		Encoder:          encoder,
	}
	req := Request{
		FileID:           file.ID,
		Path:             file.Path,
		Mtime:            file.Mtime,
		Container:        ContainerFromPath(file.Path, file.Container),
		VideoCodec:       file.VideoCodec,
		Width:            file.Width,
		Height:           file.Height,
		HDR:              file.HDR,
		DurationMS:       file.DurationMS,
		Audio:            audioStreams(file.AudioStreams),
		Subtitles:        subtitleStreams(file.SubtitleStreams),
		AudioIndex:       payload.AudioIndex,
		BurnSubtitle:     payload.BurnSubtitle,
		BurnSubtitlePath: payload.BurnSubtitlePath,
		MaxHeight:        payload.MaxHeight,
	}
	part := payload.OutPath + ".part"
	if err := os.MkdirAll(filepath.Dir(payload.OutPath), 0o755); err != nil {
		return err
	}
	plan, err := BuildVariantPlanFor(req, spec, encoder, hwaccel, part)
	if err != nil {
		return err
	}
	_ = st.SetVariantState(ctx, payload.VariantID, "running", part, 0, file.DurationMS, job.ID, "")
	report(0, "converting "+filepath.Base(file.Path))

	cmd := exec.CommandContext(ctx, ffmpeg, plan.Args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scan := bufio.NewScanner(stdout)
	lastPct := -1.0
	for scan.Scan() {
		line := scan.Text()
		var raw string
		switch {
		case strings.HasPrefix(line, "out_time_us="):
			raw = strings.TrimPrefix(line, "out_time_us=")
		case strings.HasPrefix(line, "out_time_ms="):
			raw = strings.TrimPrefix(line, "out_time_ms=")
		default:
			continue
		}
		us, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || file.DurationMS <= 0 {
			continue
		}
		pct := float64(us/1000) / float64(file.DurationMS) * 100
		if pct < 0 {
			pct = 0
		}
		if pct > 99 {
			pct = 99
		}
		if pct-lastPct >= 1 {
			lastPct = pct
			report(pct, fmt.Sprintf("converting %s (%.0f%%)", filepath.Base(file.Path), pct))
		}
	}
	if err := cmd.Wait(); err != nil {
		_ = os.Remove(part)
		msg := strings.TrimSpace(stderr.String())
		_ = st.SetVariantState(ctx, payload.VariantID, "failed", "", 0, file.DurationMS, job.ID, msg)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ffmpeg: %s", msg)
	}
	stt, err := os.Stat(part)
	if err != nil {
		return err
	}
	if err := os.Rename(part, payload.OutPath); err != nil {
		return err
	}
	if err := st.SetVariantState(ctx, payload.VariantID, "ready", payload.OutPath, stt.Size(), file.DurationMS, job.ID, ""); err != nil {
		return err
	}
	report(100, item.Title+": variant ready")
	return nil
}

func audioStreams(in []store.AudioStream) []AudioStream {
	out := make([]AudioStream, 0, len(in))
	for _, a := range in {
		out = append(out, AudioStream{
			Index: a.Index, Codec: a.Codec, Channels: a.Channels,
			Language: a.Language, Title: a.Title, Default: a.Default,
		})
	}
	return out
}

func subtitleStreams(in []store.SubtitleStream) []SubtitleStream {
	out := make([]SubtitleStream, 0, len(in))
	for _, s := range in {
		out = append(out, SubtitleStream{
			Index: s.Index, Codec: s.Codec, Language: s.Language, Title: s.Title, Forced: s.Forced,
		})
	}
	return out
}

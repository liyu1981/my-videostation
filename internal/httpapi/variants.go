package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/liyu1981/myvideostation/internal/playback"
	"github.com/liyu1981/myvideostation/internal/store"
)

// prepareDTO is the state of a background playback preparation (ADR 0006).
type prepareDTO struct {
	State     string   `json:"state"` // direct|queued|running|ready|failed
	URL       string   `json:"url,omitempty"`
	JobID     int64    `json:"jobId,omitempty"`
	VariantID int64    `json:"variantId,omitempty"`
	Progress  float64  `json:"progress,omitempty"`
	Error     string   `json:"error,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
}

// handlePreparePlayback returns a direct-play URL or queues/returns the cached
// variant for the requested profile.
func (s *Server) handlePreparePlayback(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	var in transcodeInput
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
			return
		}
	}
	item, err := s.store.GetItem(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	files, err := s.store.ItemFiles(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	var file *store.File
	for i := range files {
		if in.FileID != 0 && files[i].ID != in.FileID {
			continue
		}
		if !files[i].Missing {
			file = &files[i]
			break
		}
	}
	if file == nil {
		writeError(w, http.StatusNotFound, "no_playable_file", "no playable file for this item")
		return
	}

	audioIndex := -1
	defaultAudio := -1
	for _, a := range file.AudioStreams {
		if defaultAudio < 0 {
			defaultAudio = a.Index
		}
		if a.Default {
			defaultAudio = a.Index
			break
		}
	}
	if in.AudioIndex != nil {
		audioIndex = *in.AudioIndex
	} else {
		audioIndex = defaultAudio
	}
	burnIndex := -1
	if in.SubtitleIndex != nil {
		burnIndex = *in.SubtitleIndex
	}
	burnPath := in.BurnSubtitlePath
	if in.ExternalSubtitle > 0 {
		sub, err := s.store.ExternalSubtitleByID(r.Context(), in.ExternalSubtitle)
		if err != nil || sub.FileID != file.ID {
			writeError(w, http.StatusNotFound, "not_found", "subtitle not found")
			return
		}
		burnPath = sub.Path
	}

	// Direct play: no burn-in, no quality cap, no explicit non-default track.
	container := playback.ContainerFromPath(file.Path, file.Container)
	audioCodecs := []string{}
	for _, a := range file.AudioStreams {
		if a.Index == audioIndex {
			audioCodecs = append(audioCodecs, a.Codec)
		}
	}
	direct := playback.EvaluateDirectPlay(container, file.VideoCodec, audioCodecs)
	if direct.OK && burnIndex < 0 && burnPath == "" && in.MaxHeight == 0 && audioIndex == defaultAudio {
		writeJSON(w, http.StatusOK, prepareDTO{
			State: "direct",
			URL:   "/api/v1/media/" + strconv.FormatInt(file.ID, 10),
		})
		return
	}
	if s.ffmpeg == "" {
		writeError(w, http.StatusServiceUnavailable, "ffmpeg_missing", "ffmpeg is required for transcoding")
		return
	}
	encoder := s.encoder
	if encoder == "" {
		encoder = playback.EncoderProfiles()[0].Name
	}
	spec := playback.VariantSpec{
		FileID:           file.ID,
		Mtime:            file.Mtime,
		Size:             file.Size,
		MaxHeight:        in.MaxHeight,
		AudioIndex:       audioIndex,
		BurnSubtitle:     burnIndex,
		BurnSubtitlePath: burnPath,
		Encoder:          encoder,
	}
	key := spec.Key()
	outPath := filepath.Join(s.cacheDir, "transcodes", strconv.FormatInt(file.ID, 10), key+".mp4")
	variant, err := s.store.VariantByKey(r.Context(), file.ID, key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	if variant == nil {
		variant, err = s.store.EnsureVariant(r.Context(), store.TranscodeVariant{
			FileID: file.ID, ItemID: item.ID, Mtime: file.Mtime, Size: file.Size,
			Key: key, Path: outPath,
		})
		if err != nil {
			s.fail(w, err)
			return
		}
	}
	if variant.State == "ready" {
		if st, err := os.Stat(variant.Path); err == nil && !st.IsDir() {
			_ = s.store.TouchVariant(r.Context(), variant.ID)
			writeJSON(w, http.StatusOK, prepareDTO{
				State: "ready", URL: variantURL(file.ID, variant.ID),
				VariantID: variant.ID,
			})
			return
		}
		// Stale row: fall through and reconvert.
		variant.State = "failed"
	}
	if (variant.State == "queued" || variant.State == "running") && variant.JobID > 0 {
		dto := prepareDTO{State: variant.State, VariantID: variant.ID, JobID: variant.JobID}
		if variant.JobID > 0 {
			if job, err := s.store.GetJob(r.Context(), variant.JobID); err == nil {
				dto.Progress = job.Progress
				if job.Status == "failed" {
					dto.State = "failed"
					dto.Error = job.Error
				}
			}
		}
		if dto.State == "queued" || dto.State == "running" {
			writeJSON(w, http.StatusOK, dto)
			return
		}
	}

	// Enqueue (or re-enqueue after failure).
	job, err := s.jobs.Enqueue(r.Context(), "transcode", playback.VariantJob{
		VariantID:        variant.ID,
		FileID:           file.ID,
		ItemID:           item.ID,
		MaxHeight:        in.MaxHeight,
		AudioIndex:       audioIndex,
		BurnSubtitle:     burnIndex,
		BurnSubtitlePath: burnPath,
		OutPath:          outPath,
		VariantKey:       key,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	_ = s.store.SetVariantState(r.Context(), variant.ID, "queued", outPath, 0, file.DurationMS, job.ID, "")
	writeJSON(w, http.StatusAccepted, prepareDTO{
		State: "queued", JobID: job.ID, VariantID: variant.ID,
		Reasons: direct.Reasons,
	})
}

func variantURL(fileID, variantID int64) string {
	return fmt.Sprintf("/api/v1/media/%d?variant=%d", fileID, variantID)
}

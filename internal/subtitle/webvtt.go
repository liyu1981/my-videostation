package subtitle

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var cueTimingRe = regexp.MustCompile(`^(\s*)(\d{1,}:\d{2}(?::\d{2})?\.\d{3})\s*-->\s*(\d{1,}:\d{2}(?::\d{2})?\.\d{3})(.*)$`)

// ShiftWebVTT moves every cue by delayMS. Cues completely before zero are
// dropped; cues crossing zero are clamped to zero. It preserves cues, settings
// and the WEBVTT header.
func ShiftWebVTT(data []byte, delayMS int) []byte {
	if delayMS == 0 || len(data) == 0 {
		return data
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	drop := false
	for _, line := range lines {
		if drop {
			if strings.TrimSpace(line) == "" {
				drop = false
				if len(out) == 0 || out[len(out)-1] != "" {
					out = append(out, "")
				}
			}
			continue
		}
		if !strings.Contains(line, "-->") {
			out = append(out, line)
			continue
		}
		m := cueTimingRe.FindStringSubmatch(line)
		if m == nil {
			out = append(out, line)
			continue
		}
		start, ok1 := parseVTTTime(m[2])
		end, ok2 := parseVTTTime(m[3])
		if !ok1 || !ok2 {
			out = append(out, line)
			continue
		}
		start += int64(delayMS)
		end += int64(delayMS)
		if end <= 0 {
			// Drop the timing line and its optional cue identifier; the cue text
			// is skipped below until the next blank line.
			if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
				out = out[:len(out)-1]
			}
			drop = true
			continue
		}
		if start < 0 {
			start = 0
		}
		out = append(out, m[1]+formatVTTTime(start)+" --> "+formatVTTTime(end)+m[4])
	}
	return []byte(strings.Join(out, "\n"))
}

func parseVTTTime(s string) (int64, bool) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var hours, minutes int
	secPart := parts[len(parts)-1]
	for i := 0; i < len(parts)-1; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return 0, false
		}
		if i == 0 && len(parts) == 3 {
			hours = n
		} else {
			minutes = n
		}
	}
	secParts := strings.SplitN(secPart, ".", 2)
	if len(secParts) != 2 || len(secParts[1]) != 3 {
		return 0, false
	}
	sec, err := strconv.Atoi(secParts[0])
	if err != nil || sec < 0 || sec > 59 {
		return 0, false
	}
	ms, err := strconv.Atoi(secParts[1])
	if err != nil || ms < 0 || ms > 999 {
		return 0, false
	}
	return int64(hours)*3600000 + int64(minutes)*60000 + int64(sec)*1000 + int64(ms), true
}

func formatVTTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	m := ms / 60000
	ms %= 60000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

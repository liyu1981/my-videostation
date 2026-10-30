// Package subtitle converts sidecar and embedded subtitle formats to WebVTT.
package subtitle

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// Track is a normalised subtitle ready to serve.
type Track struct {
	Format   string // srt, ass, ssa, vtt
	Language string
	Label    string
	Data     []byte // UTF-8, original format
}

var (
	reSRTTime  = regexp.MustCompile(`(\d{1,2}:\d{2}:\d{2}),(\d{1,3})`)
	reASSTag   = regexp.MustCompile(`\{[^}]*\}`)
	reASSTime  = regexp.MustCompile(`^(\d+):(\d{1,2}):(\d{1,2})[.,](\d{1,2})$`)
	reHTMLTags = regexp.MustCompile(`<[^>]+>`)
)

// ToWebVTT converts SRT/ASS/SSA/VTT bytes (any supported charset) into WebVTT.
func ToWebVTT(format string, data []byte) ([]byte, error) {
	data = Decode(data)
	body := strings.ReplaceAll(string(data), "\r\n", "\n")
	body = strings.TrimPrefix(body, "\ufeff")

	switch strings.ToLower(format) {
	case "vtt", "webvtt":
		if !strings.HasPrefix(body, "WEBVTT") {
			body = "WEBVTT\n\n" + body
		}
		return []byte(body), nil
	case "srt", "":
		return []byte(srtToVTT(body)), nil
	case "ass", "ssa":
		return []byte(assToVTT(body)), nil
	default:
		return nil, fmt.Errorf("unsupported subtitle format %q", format)
	}
}

// Decode returns UTF-8 text, guessing GB18030 / Big5 for legacy CJK files.
func Decode(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	if utf8.Valid(data) {
		return data
	}
	for _, dec := range []*encoding.Decoder{
		simplifiedchinese.GB18030.NewDecoder(),
		traditionalchinese.Big5.NewDecoder(),
	} {
		out, _, err := transform.Bytes(dec, data)
		if err != nil {
			continue
		}
		if utf8.Valid(out) && !strings.Contains(string(out), "\uFFFD") {
			return out
		}
	}
	return data
}

func srtToVTT(s string) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	s = reSRTTime.ReplaceAllString(s, "$1.$2")
	b.WriteString(normalizeCues(s))
	return b.String()
}

// normalizeCues keeps SRT cue blocks (index lines are allowed but dropped).
func normalizeCues(s string) string {
	blocks := strings.Split(strings.TrimSpace(s), "\n\n")
	var out strings.Builder
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		// Drop a leading numeric cue index.
		if _, err := fmt.Sscanf(lines[0], "%d", new(int)); err == nil && len(lines) > 1 {
			lines = lines[1:]
		}
		if len(lines) == 0 {
			continue
		}
		out.WriteString(strings.Join(lines, "\n"))
		out.WriteString("\n\n")
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

func assToVTT(s string) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")

	fields := []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}
	startIdx, endIdx, textIdx := 1, 2, 9
	formatSeen := false

	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "Format:") && !formatSeen {
			parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(line, "Format:")), ",")
			for i, p := range parts {
				switch strings.ToLower(strings.TrimSpace(p)) {
				case "start":
					startIdx = i
				case "end":
					endIdx = i
				case "text":
					textIdx = i
				}
			}
			formatSeen = true
			continue
		}
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "Dialogue:"))
		parts := strings.SplitN(payload, ",", len(fields))
		if len(parts) <= startIdx || len(parts) <= endIdx || len(parts) <= textIdx {
			continue
		}
		start, ok1 := assTimestamp(parts[startIdx])
		end, ok2 := assTimestamp(parts[endIdx])
		if !ok1 || !ok2 {
			continue
		}
		text := parts[textIdx]
		text = reASSTag.ReplaceAllString(text, "")
		text = strings.ReplaceAll(text, `\N`, "\n")
		text = strings.ReplaceAll(text, `\n`, "\n")
		text = strings.ReplaceAll(text, `\h`, " ")
		text = escapeVTT(text)
		if strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", start, end, text)
	}
	return b.String()
}

func assTimestamp(v string) (string, bool) {
	m := reASSTime.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return "", false
	}
	var h, mm, ss, cs int
	fmt.Sscanf(m[1], "%d", &h)
	fmt.Sscanf(m[2], "%d", &mm)
	fmt.Sscanf(m[3], "%d", &ss)
	fmt.Sscanf(m[4], "%d", &cs)
	ms := cs * 10
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, mm, ss, ms), true
}

func escapeVTT(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return reHTMLTags.ReplaceAllString(s, "")
}

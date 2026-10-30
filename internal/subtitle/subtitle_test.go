package subtitle

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func TestSRTToVTT(t *testing.T) {
	srt := "\ufeff1\r\n00:00:01,000 --> 00:00:03,500\r\nHello <i>world</i>\r\n\r\n2\r\n00:00:04,000 --> 00:00:06,000\r\nSecond line\r\nline two\r\n"
	out, err := ToWebVTT("srt", []byte(srt))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "WEBVTT\n\n") {
		t.Fatalf("missing header: %q", s[:20])
	}
	if !strings.Contains(s, "00:00:01.000 --> 00:00:03.500") {
		t.Errorf("timestamp not converted:\n%s", s)
	}
	if !strings.Contains(s, "Hello <i>world</i>") {
		t.Errorf("text lost:\n%s", s)
	}
	if !strings.Contains(s, "Second line\nline two") {
		t.Errorf("multi-line cue lost:\n%s", s)
	}
	if strings.Contains(s, "\r") {
		t.Error("CRLF not normalised")
	}
}

func TestASSToVTT(t *testing.T) {
	ass := `[Script Info]
Title: test
ScriptType: v4.00+

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.23,0:00:03.45,Default,,0,0,0,,{\i1}Hello{\i0}\Nworld
Dialogue: 0,0:00:04.00,0:00:05.50,Default,,0,0,0,,Plain line
`
	out, err := ToWebVTT("ass", []byte(ass))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "WEBVTT") {
		t.Fatalf("missing header: %q", s)
	}
	if !strings.Contains(s, "00:00:01.230 --> 00:00:03.450") {
		t.Errorf("start/end conversion wrong:\n%s", s)
	}
	if !strings.Contains(s, "Hello\nworld") {
		t.Errorf("tags/line breaks not handled:\n%s", s)
	}
	if !strings.Contains(s, "Plain line") {
		t.Errorf("second cue missing:\n%s", s)
	}
	if strings.Contains(s, "{\\i1}") {
		t.Errorf("override tags leaked:\n%s", s)
	}
}

func TestGB18030Decoding(t *testing.T) {
	text := "1\r\n00:00:01,000 --> 00:00:02,000\r\n你好，世界\r\n"
	enc, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	out, err := ToWebVTT("srt", enc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "你好，世界") {
		t.Fatalf("GBK not decoded: %q", string(out))
	}
}

func TestVTTAndErrors(t *testing.T) {
	out, err := ToWebVTT("vtt", []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nhi\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "WEBVTT") != 1 {
		t.Errorf("double header: %q", out)
	}
	if _, err := ToWebVTT("pgs", nil); err == nil {
		t.Error("expected error for unsupported format")
	}
}

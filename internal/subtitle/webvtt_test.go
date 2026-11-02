package subtitle

import "testing"

func TestShiftWebVTT(t *testing.T) {
	in := "WEBVTT\n\n00:00:01.000 --> 00:00:02.500 align:start\nHello\n\n00:00:03.000 --> 00:00:04.000\nWorld\n"
	got := string(ShiftWebVTT([]byte(in), 1000))
	want := "WEBVTT\n\n00:00:02.000 --> 00:00:03.500 align:start\nHello\n\n00:00:04.000 --> 00:00:05.000\nWorld\n"
	if got != want {
		t.Fatalf("ShiftWebVTT() = %q, want %q", got, want)
	}
}

func TestShiftWebVTTNegativeClampsAndDrops(t *testing.T) {
	in := "WEBVTT\n\n00:00:00.500 --> 00:00:01.500\nA\n\n00:00:03.000 --> 00:00:04.000\nB\n"
	got := string(ShiftWebVTT([]byte(in), -1000))
	want := "WEBVTT\n\n00:00:00.000 --> 00:00:00.500\nA\n\n00:00:02.000 --> 00:00:03.000\nB\n"
	if got != want {
		t.Fatalf("ShiftWebVTT() = %q, want %q", got, want)
	}
	if got := string(ShiftWebVTT([]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nA\n"), -5000)); got != "WEBVTT\n" {
		t.Fatalf("fully negative cue not dropped: %q", got)
	}
}

func TestShiftWebVTTDropsIdentifierWithCue(t *testing.T) {
	in := "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:02.000\nGone\n\n00:00:06.000 --> 00:00:07.000\nKeep\n"
	got := string(ShiftWebVTT([]byte(in), -5000))
	want := "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nKeep\n"
	if got != want {
		t.Fatalf("ShiftWebVTT() = %q, want %q", got, want)
	}
}

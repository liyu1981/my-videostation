package playback

import (
	"strings"
	"testing"
)

func variantRequest(video, audio string) Request {
	return Request{
		FileID: 7, Path: "/media/x.mkv", Mtime: 100, Container: "matroska",
		VideoCodec: video, Width: 1920, Height: 1080, DurationMS: 60000,
		Audio:        []AudioStream{{Index: 2, Codec: audio, Channels: 2, Default: true}},
		AudioIndex:   2,
		BurnSubtitle: -1,
	}
}

func variantSpec() VariantSpec {
	return VariantSpec{FileID: 7, Mtime: 100, Size: 1000, MaxHeight: 0, AudioIndex: 2, BurnSubtitle: -1, Encoder: "libx264"}
}

func TestBuildVariantPlanCopiesSupportedStreams(t *testing.T) {
	p, err := BuildVariantPlanFor(variantRequest("h264", "aac"), variantSpec(), "libx264", "", "/tmp/out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !p.CopyVideo || !p.CopyAudio {
		t.Fatalf("copy flags = %v/%v", p.CopyVideo, p.CopyAudio)
	}
	args := strings.Join(p.Args, " ")
	for _, want := range []string{"-c:v copy", "-c:a copy", "-movflags +faststart", "-f mp4", "/tmp/out.mp4"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing %q in %v", want, p.Args)
		}
	}
	for _, bad := range []string{"-hls", "-ss", "-readrate", "-force_key_frames"} {
		if strings.Contains(args, bad) {
			t.Fatalf("variant must not contain %q: %v", bad, p.Args)
		}
	}
}

func TestBuildVariantPlanTranscodesOnlyUnsupportedStream(t *testing.T) {
	p, err := BuildVariantPlanFor(variantRequest("h264", "ac3"), variantSpec(), "libx264", "", "/tmp/out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(p.Args, " ")
	if !p.CopyVideo || p.CopyAudio {
		t.Fatalf("copy flags = %v/%v", p.CopyVideo, p.CopyAudio)
	}
	if !strings.Contains(args, "-c:v copy") || !strings.Contains(args, "-c:a aac") {
		t.Fatalf("expected video copy + AAC: %v", p.Args)
	}

	p, err = BuildVariantPlanFor(variantRequest("hevc", "aac"), variantSpec(), "libx264", "", "/tmp/out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	args = strings.Join(p.Args, " ")
	if p.CopyVideo || !p.CopyAudio {
		t.Fatalf("copy flags = %v/%v", p.CopyVideo, p.CopyAudio)
	}
	if !strings.Contains(args, "-c:v libx264") || !strings.Contains(args, "-c:a copy") {
		t.Fatalf("expected video encode + audio copy: %v", p.Args)
	}
}

func TestBuildVariantPlanTranscodesBothUnsupported(t *testing.T) {
	p, err := BuildVariantPlanFor(variantRequest("hevc", "ac3"), variantSpec(), "libx264", "", "/tmp/out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(p.Args, " ")
	if p.CopyVideo || p.CopyAudio {
		t.Fatalf("copy flags = %v/%v", p.CopyVideo, p.CopyAudio)
	}
	if !strings.Contains(args, "-c:v libx264") || !strings.Contains(args, "-c:a aac") {
		t.Fatalf("expected full transcode: %v", p.Args)
	}
}

func TestVariantKeyChangesWithProfile(t *testing.T) {
	base := variantSpec()
	a := base.Key()
	if a == "" || len(a) != 32 {
		t.Fatalf("key = %q", a)
	}
	if base.Key() != a {
		t.Fatal("key is not stable")
	}
	for _, mutate := range []func(*VariantSpec){
		func(s *VariantSpec) { s.MaxHeight = 720 },
		func(s *VariantSpec) { s.AudioIndex = 3 },
		func(s *VariantSpec) { s.BurnSubtitle = 4 },
		func(s *VariantSpec) { s.Encoder = "h264_nvenc" },
		func(s *VariantSpec) { s.Mtime = 101 },
	} {
		s := base
		mutate(&s)
		if s.Key() == a {
			t.Fatalf("key did not change after mutation: %+v", s)
		}
	}
}

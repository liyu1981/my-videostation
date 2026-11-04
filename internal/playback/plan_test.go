package playback

import (
	"strings"
	"testing"
)

func TestEvaluateDirectPlay(t *testing.T) {
	cases := []struct {
		name      string
		container string
		video     string
		audio     []string
		want      bool
		wantIn    string
	}{
		{"mp4 h264 aac", "mp4", "h264", []string{"aac"}, true, ""},
		{"webm vp9 opus", "webm", "vp9", []string{"opus"}, true, ""},
		{"mkv h264 aac", "mkv", "h264", []string{"aac"}, false, "mkv container"},
		{"hevc video", "mp4", "hevc", []string{"aac"}, false, "HEVC video"},
		{"ac3 audio", "mkv", "h264", []string{"ac3"}, false, ""},
		{"empty facts", "", "", nil, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := EvaluateDirectPlay(tc.container, tc.video, tc.audio)
			if d.OK != tc.want {
				t.Fatalf("OK = %v, want %v (reasons %v)", d.OK, tc.want, d.Reasons)
			}
			if tc.wantIn != "" {
				joined := strings.Join(d.Reasons, ", ")
				if !strings.Contains(joined, tc.wantIn) {
					t.Fatalf("reasons %q do not contain %q", joined, tc.wantIn)
				}
			}
		})
	}
}

func contains(args []string, pair ...string) bool {
	for i := 0; i+len(pair) <= len(args); i++ {
		ok := true
		for j, want := range pair {
			if args[i+j] != want {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func argValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func baseRequest() Request {
	return Request{
		FileID:     7,
		Path:       "/media/movie.mkv",
		Container:  "mkv",
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		DurationMS: 600000,
		Audio:      []AudioStream{{Index: 1, Codec: "aac", Channels: 2}},
		AudioIndex: -1, BurnSubtitle: -1,
		OutDir: "/tmp/sess",
	}
}

func TestBuildPlanRemux(t *testing.T) {
	p, err := BuildPlan(baseRequest(), options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeRemux {
		t.Fatalf("mode = %s, want remux", p.Mode)
	}
	if !contains(p.Args, "-c:v", "copy") || !contains(p.Args, "-c:a", "copy") {
		t.Fatalf("expected stream copy, got %v", p.Args)
	}
	if contains(p.Args, "-vf") {
		t.Fatalf("remux must not filter: %v", p.Args)
	}
	if contains(p.Args, "-force_key_frames", "expr:gte(t,n_forced*4)") {
		t.Fatalf("remux must not re-keyframe: %v", p.Args)
	}
	if !contains(p.Args, "-f", "hls") || !contains(p.Args, "-hls_segment_type", "fmp4") {
		t.Fatalf("missing HLS fMP4 args: %v", p.Args)
	}
	if got := p.Args[len(p.Args)-1]; got != "/tmp/sess/index.m3u8" {
		t.Fatalf("last arg = %q, want playlist path", got)
	}
}

func TestBuildPlanTranscodeVideo(t *testing.T) {
	req := baseRequest()
	req.VideoCodec = "hevc"
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeTranscode {
		t.Fatalf("mode = %s, want transcode", p.Mode)
	}
	if !contains(p.Args, "-c:v", "libx264") || !contains(p.Args, "-c:a", "copy") {
		t.Fatalf("expected video transcode + audio copy: %v", p.Args)
	}
	if !contains(p.Args, "-force_key_frames", "expr:gte(t,n_forced*4)") {
		t.Fatalf("missing keyframe pinning: %v", p.Args)
	}
	if !strings.Contains(strings.Join(p.Reasons, ","), "HEVC video") {
		t.Fatalf("missing reason: %v", p.Reasons)
	}
}

func TestBuildPlanTranscodeAudioOnly(t *testing.T) {
	req := baseRequest()
	req.Audio = []AudioStream{{Index: 2, Codec: "dts", Channels: 6}}
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeTranscode {
		t.Fatalf("mode = %s, want transcode", p.Mode)
	}
	if !contains(p.Args, "-c:v", "copy") {
		t.Fatalf("video should stay copied: %v", p.Args)
	}
	if !contains(p.Args, "-c:a", "aac") || argValue(p.Args, "-ac") != "2" {
		t.Fatalf("expected 6ch DTS downmixed to AAC stereo: %v", p.Args)
	}
}

func TestBuildPlanAudioSelection(t *testing.T) {
	req := baseRequest()
	req.Audio = []AudioStream{
		{Index: 1, Codec: "aac", Channels: 2, Default: true},
		{Index: 3, Codec: "ac3", Channels: 6},
	}
	req.AudioIndex = 3
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.AudioIndex != 3 || !contains(p.Args, "-map", "0:3") {
		t.Fatalf("expected stream 3 mapped, got index %d args %v", p.AudioIndex, p.Args)
	}
	if !contains(p.Args, "-c:a", "aac") {
		t.Fatalf("AC3 should be transcoded: %v", p.Args)
	}
}

func TestBuildPlanScalingAndSeek(t *testing.T) {
	req := baseRequest()
	req.VideoCodec = "hevc"
	req.MaxHeight = 720
	req.StartMS = 61500
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Height != 720 || p.Width != 1280 {
		t.Fatalf("size = %dx%d, want 1280x720", p.Width, p.Height)
	}
	if !contains(p.Args, "-vf", "scale=-2:720,setsar=1") {
		t.Fatalf("expected scale filter: %v", p.Args)
	}
	if argValue(p.Args, "-ss") != "61.500" {
		t.Fatalf("-ss = %q", argValue(p.Args, "-ss"))
	}
	// -ss must come before -i for fast input seeking.
	ss, in := -1, -1
	for i, a := range p.Args {
		if a == "-ss" {
			ss = i
		}
		if a == "-i" {
			in = i
		}
	}
	if ss < 0 || in < 0 || ss > in {
		t.Fatalf("-ss (%d) must precede -i (%d)", ss, in)
	}
	if p.DurationMS != 600000-61500 {
		t.Fatalf("duration = %d", p.DurationMS)
	}
}

func TestBuildPlanDownscaleForcesTranscode(t *testing.T) {
	req := baseRequest() // h264/aac in mkv: remux by default
	req.MaxHeight = 480
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeTranscode || !contains(p.Args, "-c:v", "libx264") {
		t.Fatalf("downscale must re-encode: %s %v", p.Mode, p.Args)
	}
	if p.Height != 480 || !contains(p.Args, "-vf", "scale=-2:480,setsar=1") {
		t.Fatalf("height = %d args = %v", p.Height, p.Args)
	}
}

func TestBuildPlanNoUpscale(t *testing.T) {
	req := baseRequest()
	req.VideoCodec = "hevc"
	req.Height = 480
	req.MaxHeight = 1080
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Height != 480 {
		t.Fatalf("height = %d, want source 480", p.Height)
	}
	if contains(p.Args, "-vf", "scale=-2:480,setsar=1") {
		t.Fatalf("no scaling needed at source height: %v", p.Args)
	}
}

func TestBuildPlanBurnInExternal(t *testing.T) {
	req := baseRequest()
	req.VideoCodec = "hevc"
	req.BurnSubtitlePath = "/media/Subs/Movie: Part 1.srt"
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(p.Args, "-c:v", "libx264") {
		t.Fatalf("burn-in requires re-encode: %v", p.Args)
	}
	vf := argValue(p.Args, "-vf")
	if !strings.Contains(vf, `subtitles=/media/Subs/Movie\: Part 1.srt`) {
		t.Fatalf("vf = %q", vf)
	}
}

func TestBuildPlanBurnInBitmap(t *testing.T) {
	req := baseRequest()
	req.VideoCodec = "h264"
	req.Subtitles = []SubtitleStream{{Index: 4, Codec: "hdmv_pgs_subtitle"}}
	req.BurnSubtitle = 4
	p, err := BuildPlan(req, options{Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeTranscode {
		t.Fatalf("bitmap burn-in cannot copy: %s", p.Mode)
	}
	graph := argValue(p.Args, "-filter_complex")
	if !strings.Contains(graph, "[0:4]overlay") || !contains(p.Args, "-map", "[vout]") {
		t.Fatalf("graph = %q args = %v", graph, p.Args)
	}
	if contains(p.Args, "-vf") {
		t.Fatalf("must not mix -vf with -filter_complex: %v", p.Args)
	}
}

func TestBuildPlanForceTranscode(t *testing.T) {
	req := baseRequest()
	req.Force = "transcode"
	p, err := BuildPlan(req, options{Encoder: "h264_nvenc"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeTranscode || !contains(p.Args, "-c:v", "h264_nvenc") {
		t.Fatalf("expected forced NVENC transcode: %s %v", p.Mode, p.Args)
	}
}

func TestBuildPlanPacing(t *testing.T) {
	req := baseRequest()
	req.VideoCodec = "hevc"
	p, err := BuildPlan(req, options{Encoder: "libx264", Paced: true, Readrate: true})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(p.Args, "-readrate", "1.0") {
		t.Fatalf("missing readrate: %v", p.Args)
	}
	p, err = BuildPlan(req, options{Encoder: "libx264", Paced: true, Readrate: false})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(p.Args, "-re") {
		t.Fatalf("missing -re fallback: %v", p.Args)
	}
	p, err = BuildPlan(req, options{Encoder: "libx264", Paced: false})
	if err != nil {
		t.Fatal(err)
	}
	if contains(p.Args, "-re") || contains(p.Args, "-readrate", "1.0") {
		t.Fatalf("unpaced plan should not throttle: %v", p.Args)
	}
}

func TestQualityLadder(t *testing.T) {
	got := QualityLadder(1080)
	want := []int{0, 720, 480, 360, 240}
	if len(got) != len(want) {
		t.Fatalf("ladder = %v", got)
	}
	for i, w := range want {
		if got[i].Height != w {
			t.Fatalf("ladder[%d] = %v, want %d", i, got[i], w)
		}
	}
	if len(QualityLadder(0)) != 1 {
		t.Fatal("unknown source height should only offer auto")
	}
}

func TestSessionKeyBucketsSeeks(t *testing.T) {
	a := baseRequest()
	a.StartMS = 42000
	b := baseRequest()
	b.StartMS = 43100
	if sessionKey(a) != sessionKey(b) {
		t.Fatal("seeks inside the same 2s bucket should reuse a session")
	}
	c := baseRequest()
	c.StartMS = 60000
	if sessionKey(a) == sessionKey(c) {
		t.Fatal("distant seeks must get a different session")
	}
	d := baseRequest()
	d.AudioIndex = 3
	if sessionKey(a) == sessionKey(d) {
		t.Fatal("audio switching must restart the session")
	}
}

func TestEvaluateDirectPlayContainerAliases(t *testing.T) {
	// ffprobe reports MP4 as "mov,mp4,m4a,3gp,3g2,mj2".
	if d := EvaluateDirectPlay("mov,mp4,m4a,3gp,3g2,mj2", "h264", []string{"aac"}); !d.OK {
		t.Fatalf("mp4 alias should direct play: %v", d.Reasons)
	}
	if got := ContainerFromPath("/media/a.mkv", "matroska,webm"); got != "matroska" {
		t.Fatalf("mkv container = %q", got)
	}
	if got := ContainerFromPath("/media/a.webm", "matroska,webm"); got != "webm" {
		t.Fatalf("webm container = %q", got)
	}
	if d := EvaluateDirectPlay(ContainerFromPath("/media/a.webm", "matroska,webm"), "vp9", []string{"opus"}); !d.OK {
		t.Fatalf("webm should direct play: %v", d.Reasons)
	}
}

package httpapi

import (
	"testing"

	"github.com/liyu1981/myvideostation/internal/store"
)

func TestCastCompatibility(t *testing.T) {
	mp4 := &store.File{Container: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodec: "h264", AudioStreams: []store.AudioStream{{Codec: "aac"}}}
	if ok, reason := castCompatible(mp4); !ok {
		t.Fatalf("mp4/h264/aac rejected: %s", reason)
	}
	mkv := &store.File{Container: "matroska,webm", VideoCodec: "h264", AudioStreams: []store.AudioStream{{Codec: "aac"}}}
	if ok, _ := castCompatible(mkv); ok {
		t.Fatal("mkv accepted")
	}
	ac3 := &store.File{Container: "mov,mp4,m4a,3gp,3g2,mj2", VideoCodec: "h264", AudioStreams: []store.AudioStream{{Codec: "ac3"}}}
	if ok, _ := castCompatible(ac3); ok {
		t.Fatal("ac3 accepted")
	}
	if ok, _ := castCompatible(&store.File{}); ok {
		t.Fatal("unknown format accepted")
	}
}

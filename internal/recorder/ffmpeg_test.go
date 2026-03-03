package recorder

import (
	"strings"
	"testing"
)

func TestBuildEncodeSegmentArgs(t *testing.T) {
	t.Parallel()

	cfg := Config{
		VideoFPS:        25,
		VideoQualityCRF: 23,
	}

	args := BuildEncodeSegmentArgs(cfg, "/tmp/frames/frame_%08d.png", 10, 42, "/tmp/out/out.mp4")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-start_number 10") {
		t.Fatalf("expected start_number in args: %s", joined)
	}
	if !strings.Contains(joined, "-frames:v 42") {
		t.Fatalf("expected frame count in args: %s", joined)
	}
	if !strings.HasSuffix(joined, "/tmp/out/out.mp4") {
		t.Fatalf("expected output path in args: %s", joined)
	}
}

func TestOutputPaths(t *testing.T) {
	t.Parallel()

	cfg := Config{
		OutputDir:    "/tmp/out",
		FileBaseName: "video",
		SessionID:    "sid",
	}

	segment := SegmentOutputPath(cfg, 7)
	if !strings.HasSuffix(segment, "/tmp/out/segment_sid_007.mp4") {
		t.Fatalf("segment output = %q", segment)
	}

	single := SingleFileOutputPath(cfg)
	if !strings.HasSuffix(single, "/tmp/out/video_sid.mp4") {
		t.Fatalf("single output = %q", single)
	}
}

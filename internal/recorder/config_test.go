package recorder

import (
	"strings"
	"testing"
)

func TestLoadConfig_EnvDefaultsAndFlagOverride(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"VNC_HOST":         "vnc.example",
		"VNC_PORT":         "5901",
		"VIDEO_FPS":        "12",
		"SEGMENT_DURATION": "120",
		"S3_BUCKET":        "bucket-a",
		"S3_PREFIX":        "captures",
	}

	getenv := func(k string) string {
		return env[k]
	}

	cfg, err := LoadConfig([]string{"--video-fps", "30", "--segment-duration", "0"}, getenv)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.VNCHost != "vnc.example" {
		t.Fatalf("VNCHost = %q, want %q", cfg.VNCHost, "vnc.example")
	}
	if cfg.VNCPort != 5901 {
		t.Fatalf("VNCPort = %d, want 5901", cfg.VNCPort)
	}
	if cfg.VideoFPS != 30 {
		t.Fatalf("VideoFPS = %d, want 30", cfg.VideoFPS)
	}
	if !cfg.SingleFileMode() {
		t.Fatal("SingleFileMode() = false, want true")
	}
	if !cfg.UploadToS3 {
		t.Fatal("UploadToS3 = false, want true (bucket set)")
	}
	if cfg.S3Prefix != "captures/" {
		t.Fatalf("S3Prefix = %q, want captures/", cfg.S3Prefix)
	}
}

func TestLoadConfig_ValidationError(t *testing.T) {
	t.Parallel()

	getenv := func(_ string) string { return "" }
	_, err := LoadConfig([]string{"--upload-s3=true"}, getenv)
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "s3 bucket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

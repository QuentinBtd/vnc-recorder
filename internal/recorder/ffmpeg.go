package recorder

import (
	"path/filepath"
	"strconv"
)

func BuildEncodeSegmentArgs(cfg Config, framesPattern string, startFrame int, frameCount int, outputPath string) []string {
	return []string{
		"-y",
		"-hide_banner",
		"-loglevel", "warning",
		"-framerate", strconv.Itoa(cfg.VideoFPS),
		"-start_number", strconv.Itoa(startFrame),
		"-i", framesPattern,
		"-frames:v", strconv.Itoa(frameCount),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", strconv.Itoa(cfg.VideoQualityCRF),
		"-pix_fmt", "yuv420p",
		outputPath,
	}
}

func SegmentOutputPath(cfg Config, segmentID int) string {
	return filepath.Join(cfg.OutputDir, "segment_"+cfg.SessionID+"_"+leftPad3(segmentID)+".mp4")
}

func SingleFileOutputPath(cfg Config) string {
	return filepath.Join(cfg.OutputDir, cfg.FileBaseName+"_"+cfg.SessionID+".mp4")

}

func leftPad3(v int) string {
	s := strconv.Itoa(v)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

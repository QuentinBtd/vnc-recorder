package recorder

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOutputDir = "/tmp/vnc-recordings"
	defaultFPS       = 25
	defaultCRF       = 23
)

type Config struct {
	VNCHost                 string
	VNCPort                 int
	VNCPassword             string
	VNCConnectMaxRetries    int
	VNCConnectRetryDelaySec int
	VideoFPS                int
	VideoQualityCRF         int
	SegmentDurationSec      int
	OutputDir               string
	FileBaseName            string
	FFmpegPath              string
	UploadToS3              bool
	S3Bucket                string
	S3Prefix                string
	AWSRegion               string
	AWSProfile              string
	SessionID               string
}

func LoadConfig(args []string, getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	cfg := Config{
		VNCHost:                 fromEnv(getenv, "VNC_HOST", "localhost"),
		VNCPort:                 fromEnvInt(getenv, "VNC_PORT", 5900),
		VNCPassword:             fromEnv(getenv, "VNC_PASSWORD", ""),
		VNCConnectMaxRetries:    fromEnvInt(getenv, "VNC_CONNECT_MAX_RETRIES", 30),
		VNCConnectRetryDelaySec: fromEnvInt(getenv, "VNC_CONNECT_RETRY_DELAY", 2),
		VideoFPS:                fromEnvInt(getenv, "VIDEO_FPS", defaultFPS),
		VideoQualityCRF:         fromEnvInt(getenv, "VIDEO_QUALITY", defaultCRF),
		SegmentDurationSec:      fromEnvInt(getenv, "SEGMENT_DURATION", 300),
		OutputDir:               fromEnv(getenv, "OUTPUT_DIR", defaultOutputDir),
		FileBaseName:            fromEnv(getenv, "FILE_BASENAME", ""),
		FFmpegPath:              fromEnv(getenv, "FFMPEG_PATH", "ffmpeg"),
		S3Bucket:                fromEnv(getenv, "S3_BUCKET", ""),
		S3Prefix:                fromEnv(getenv, "S3_PREFIX", ""),
		AWSRegion:               fromEnv(getenv, "AWS_REGION", ""),
		AWSProfile:              fromEnv(getenv, "AWS_PROFILE", ""),
	}

	if raw := getenv("UPLOAD_S3"); raw != "" {
		cfg.UploadToS3 = parseBool(raw, false)
	} else {
		cfg.UploadToS3 = cfg.S3Bucket != ""
	}

	if cfg.FileBaseName == "" {
		cfg.FileBaseName = "recording"
	}

	fs := flag.NewFlagSet("vnc-recorder", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.VNCHost, "vnc-host", cfg.VNCHost, "VNC host")
	fs.IntVar(&cfg.VNCPort, "vnc-port", cfg.VNCPort, "VNC port")
	fs.StringVar(&cfg.VNCPassword, "vnc-password", cfg.VNCPassword, "VNC password")
	fs.IntVar(&cfg.VNCConnectMaxRetries, "vnc-connect-max-retries", cfg.VNCConnectMaxRetries, "VNC connection retries")
	fs.IntVar(&cfg.VNCConnectRetryDelaySec, "vnc-connect-retry-delay", cfg.VNCConnectRetryDelaySec, "Delay between retries (seconds)")
	fs.IntVar(&cfg.VideoFPS, "video-fps", cfg.VideoFPS, "Output FPS")
	fs.IntVar(&cfg.VideoQualityCRF, "video-quality", cfg.VideoQualityCRF, "x264 CRF quality (0-51)")
	fs.IntVar(&cfg.SegmentDurationSec, "segment-duration", cfg.SegmentDurationSec, "Segment duration in seconds, 0 for a single file")
	fs.StringVar(&cfg.OutputDir, "output-dir", cfg.OutputDir, "Local output directory")
	fs.StringVar(&cfg.FileBaseName, "file-basename", cfg.FileBaseName, "Output basename")
	fs.StringVar(&cfg.FFmpegPath, "ffmpeg-path", cfg.FFmpegPath, "ffmpeg binary path")
	fs.BoolVar(&cfg.UploadToS3, "upload-s3", cfg.UploadToS3, "Upload outputs to S3")
	fs.StringVar(&cfg.S3Bucket, "s3-bucket", cfg.S3Bucket, "S3 bucket")
	fs.StringVar(&cfg.S3Prefix, "s3-prefix", cfg.S3Prefix, "S3 key prefix")
	fs.StringVar(&cfg.AWSRegion, "aws-region", cfg.AWSRegion, "AWS region override")
	fs.StringVar(&cfg.AWSProfile, "aws-profile", cfg.AWSProfile, "AWS shared profile override")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.S3Prefix = normalizePrefix(cfg.S3Prefix)
	cfg.SessionID = time.Now().Format("20060102_150405") + "_" + strconv.Itoa(os.Getpid())

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.VNCHost) == "" {
		return errors.New("vnc host cannot be empty")
	}
	if c.VNCPort <= 0 || c.VNCPort > 65535 {
		return fmt.Errorf("invalid vnc port: %d", c.VNCPort)
	}
	if c.VideoFPS <= 0 {
		return fmt.Errorf("video fps must be > 0, got %d", c.VideoFPS)
	}
	if c.VideoQualityCRF < 0 || c.VideoQualityCRF > 51 {
		return fmt.Errorf("video quality must be between 0 and 51, got %d", c.VideoQualityCRF)
	}
	if c.SegmentDurationSec < 0 {
		return fmt.Errorf("segment duration cannot be negative, got %d", c.SegmentDurationSec)
	}
	if strings.TrimSpace(c.OutputDir) == "" {
		return errors.New("output directory cannot be empty")
	}
	if c.VNCConnectMaxRetries < 1 {
		return fmt.Errorf("vnc connect max retries must be >= 1, got %d", c.VNCConnectMaxRetries)
	}
	if c.VNCConnectRetryDelaySec < 1 {
		return fmt.Errorf("vnc connect retry delay must be >= 1, got %d", c.VNCConnectRetryDelaySec)
	}
	if c.UploadToS3 && strings.TrimSpace(c.S3Bucket) == "" {
		return errors.New("s3 bucket is required when upload-s3 is enabled")
	}

	return nil
}

func (c Config) SingleFileMode() bool {
	return c.SegmentDurationSec == 0
}

func fromEnv(getenv func(string) string, key string, fallback string) string {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func fromEnvInt(getenv func(string) string, key string, fallback int) int {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func parseBool(raw string, fallback bool) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

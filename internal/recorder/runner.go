package recorder

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	vnc "github.com/amitbet/vnc2video"
)

type Runner struct {
	cfg    Config
	logger *log.Logger
}

func NewRunner(cfg Config, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.Default()
	}
	return &Runner{cfg: cfg, logger: logger}
}

func (r *Runner) Run(ctx context.Context) error {
	ffmpegPath, err := resolveFFmpegPath(r.cfg.FFmpegPath)
	if err != nil {
		return err
	}
	r.cfg.FFmpegPath = ffmpegPath
	if err := os.MkdirAll(r.cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	framesDir, err := os.MkdirTemp("", "vnc-frames-")
	if err != nil {
		return fmt.Errorf("create frames dir: %w", err)
	}
	defer os.RemoveAll(framesDir)

	uploader, err := newUploader(ctx, r.cfg)
	if err != nil {
		return fmt.Errorf("init uploader: %w", err)
	}

	return r.captureAndEncode(ctx, framesDir, uploader)
}

func (r *Runner) captureAndEncode(ctx context.Context, framesDir string, uploader Uploader) error {
	framePattern := filepath.Join(framesDir, "frame_%08d.png")
	segmentStartFrame := 0
	frameCount := 0
	segmentID := 0
	segmentStart := time.Now()

	session, err := r.connectWithRetry(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	ticker := time.NewTicker(time.Second / time.Duration(r.cfg.VideoFPS))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := flushPendingSegment(frameCount, segmentStartFrame, func() error {
				return r.encodeAndUpload(uploader, framePattern, segmentStartFrame, frameCount-segmentStartFrame, segmentID)
			}); err != nil {
				return err
			}
			return nil
		case err := <-session.errorCh:
			r.logger.Printf("vnc stream error: %v, reconnecting", err)
			session.Close()
			session, err = r.connectWithRetry(ctx)
			if err != nil {
				if flushErr := flushPendingSegment(frameCount, segmentStartFrame, func() error {
					return r.encodeAndUpload(uploader, framePattern, segmentStartFrame, frameCount-segmentStartFrame, segmentID)
				}); flushErr != nil {
					return flushErr
				}
				if isCtxDoneErr(err) {
					return nil
				}
				return err
			}
		case msg := <-session.serverMessage:
			if msg.Type() == vnc.FramebufferUpdateMsgType {
				req := &vnc.FramebufferUpdateRequest{
					Inc:    1,
					X:      0,
					Y:      0,
					Width:  session.conn.Width(),
					Height: session.conn.Height(),
				}
				select {
				case session.clientMessage <- req:
				default:
				}
			}
		case <-ticker.C:
			framePath := filepath.Join(framesDir, fmt.Sprintf("frame_%08d.png", frameCount))
			if err := writeFrame(framePath, session.conn.Canvas.Image); err != nil {
				return fmt.Errorf("write frame %d: %w", frameCount, err)
			}
			frameCount++

			if !r.cfg.SingleFileMode() && time.Since(segmentStart) >= time.Duration(r.cfg.SegmentDurationSec)*time.Second {
				if frameCount > segmentStartFrame {
					if err := r.encodeAndUpload(uploader, framePattern, segmentStartFrame, frameCount-segmentStartFrame, segmentID); err != nil {
						return err
					}
				}
				segmentID++
				segmentStartFrame = frameCount
				segmentStart = time.Now()
			}
		}
	}
}

func (r *Runner) connectWithRetry(ctx context.Context) (*vncSession, error) {
	var lastErr error
	for attempt := 1; attempt <= r.cfg.VNCConnectMaxRetries; attempt++ {
		session, err := connectVNC(ctx, r.cfg)
		if err == nil {
			r.logger.Printf("connected to VNC %s:%d", r.cfg.VNCHost, r.cfg.VNCPort)
			return session, nil
		}
		lastErr = err
		r.logger.Printf("vnc connect failed (%d/%d): %v", attempt, r.cfg.VNCConnectMaxRetries, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(r.cfg.VNCConnectRetryDelaySec) * time.Second):
		}
	}
	return nil, fmt.Errorf("cannot connect to vnc after %d retries: %w", r.cfg.VNCConnectMaxRetries, lastErr)
}

func (r *Runner) encodeAndUpload(uploader Uploader, framePattern string, startFrame int, frameCount int, segmentID int) error {
	if frameCount <= 0 {
		return nil
	}

	outputPath := SegmentOutputPath(r.cfg, segmentID)
	if r.cfg.SingleFileMode() {
		outputPath = SingleFileOutputPath(r.cfg)
	}

	args := BuildEncodeSegmentArgs(r.cfg, framePattern, startFrame, frameCount, outputPath)
	cmd := exec.Command(r.cfg.FFmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("encode output %s: %w", filepath.Base(outputPath), err)
	}

	r.logger.Printf("created %s", filepath.Base(outputPath))

	if r.cfg.UploadToS3 {
		if err := uploader.Upload(context.Background(), outputPath); err != nil {
			return fmt.Errorf("upload %s: %w", filepath.Base(outputPath), err)
		}
		r.logger.Printf("uploaded %s", filepath.Base(outputPath))
		if err := os.Remove(outputPath); err != nil {
			r.logger.Printf("warn: cannot remove uploaded file %s: %v", outputPath, err)
		}
	}

	return nil
}

func writeFrame(path string, src image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	bounds := src.Bounds()
	copyFrame := image.NewRGBA(bounds)
	draw.Draw(copyFrame, bounds, src, bounds.Min, draw.Src)

	if err := png.Encode(f, copyFrame); err != nil {
		return err
	}
	return nil
}

func ensureFFmpegAvailable(ffmpegPath string) error {
	_, err := resolveFFmpegPath(ffmpegPath)
	return err
}

func resolveFFmpegPath(ffmpegPath string) (string, error) {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if p, err := exec.LookPath(ffmpegPath); err == nil {
		return p, nil
	}
	if ffmpegPath == "ffmpeg" {
		if _, err := os.Stat("/ffmpeg"); err == nil {
			return "/ffmpeg", nil
		}
	}
	return "", fmt.Errorf("ffmpeg binary %q not found in image/path", ffmpegPath)
}

func isCtxDoneErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func flushPendingSegment(frameCount int, segmentStartFrame int, encode func() error) error {
	if frameCount <= segmentStartFrame {
		return nil
	}
	return encode()
}

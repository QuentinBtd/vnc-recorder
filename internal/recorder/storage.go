package recorder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Uploader interface {
	Upload(ctx context.Context, localPath string) error
}

type noOpUploader struct{}

func (noOpUploader) Upload(_ context.Context, _ string) error { return nil }

type s3Uploader struct {
	bucket string
	prefix string
	up     *manager.Uploader
}

func newUploader(ctx context.Context, cfg Config) (Uploader, error) {
	if !cfg.UploadToS3 {
		return noOpUploader{}, nil
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	return &s3Uploader{
		bucket: cfg.S3Bucket,
		prefix: cfg.S3Prefix,
		up:     manager.NewUploader(client),
	}, nil
}

func (u *s3Uploader) Upload(ctx context.Context, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	key := buildS3Key(u.prefix, filepath.Base(localPath))
	_, err = u.up.Upload(ctx, &s3.PutObjectInput{
		Bucket: &u.bucket,
		Key:    &key,
		Body:   f,
	})
	if err != nil {
		return err
	}

	return nil
}

func buildS3Key(prefix string, filename string) string {
	if prefix == "" {
		return filename
	}
	return prefix + filename
}

func listStableFiles(pattern string, now time.Time, settle time.Duration, alreadyUploaded map[string]struct{}) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	ready := make([]string, 0, len(matches))
	for _, match := range matches {
		if _, done := alreadyUploaded[match]; done {
			continue
		}
		st, err := os.Stat(match)
		if err != nil {
			continue
		}
		if now.Sub(st.ModTime()) >= settle {
			ready = append(ready, match)
		}
	}

	return ready, nil
}

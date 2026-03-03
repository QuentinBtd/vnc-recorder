package recorder

import (
	"errors"
	"testing"
)

func TestEnsureFFmpegAvailable(t *testing.T) {
	t.Parallel()

	if err := ensureFFmpegAvailable("go"); err != nil {
		t.Fatalf("ensureFFmpegAvailable(go) error = %v", err)
	}

	if err := ensureFFmpegAvailable("definitely-not-an-existing-binary"); err == nil {
		t.Fatal("ensureFFmpegAvailable() error = nil, want error")
	}
}

func TestFlushPendingSegment(t *testing.T) {
	t.Parallel()

	called := false
	err := flushPendingSegment(10, 10, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("flushPendingSegment() error = %v", err)
	}
	if called {
		t.Fatal("encode should not be called when no pending frame")
	}

	called = false
	err = flushPendingSegment(11, 10, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("flushPendingSegment() error = %v", err)
	}
	if !called {
		t.Fatal("encode should be called when pending frames exist")
	}
}

func TestIsUnsupportedEncodingErr(t *testing.T) {
	t.Parallel()

	if !isUnsupportedEncodingErr(errors.New("unsupported encoding EncodingType(393216)")) {
		t.Fatal("expected unsupported encoding error to be detected")
	}
}

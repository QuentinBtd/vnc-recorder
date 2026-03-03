package recorder

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBuildS3Key(t *testing.T) {
	t.Parallel()

	if got := buildS3Key("", "a.mp4"); got != "a.mp4" {
		t.Fatalf("buildS3Key() = %q", got)
	}
	if got := buildS3Key("prefix/", "a.mp4"); got != "prefix/a.mp4" {
		t.Fatalf("buildS3Key() = %q", got)
	}
}

func TestListStableFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	old := filepath.Join(dir, "seg_000.mp4")
	newer := filepath.Join(dir, "seg_001.mp4")

	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	oldTime := now.Add(-20 * time.Second)
	newTime := now.Add(-1 * time.Second)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	ready, err := listStableFiles(filepath.Join(dir, "seg_*.mp4"), now, 3*time.Second, map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{old}
	if !reflect.DeepEqual(ready, want) {
		t.Fatalf("ready=%v want=%v", ready, want)
	}
}

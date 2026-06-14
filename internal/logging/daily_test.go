package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyWriterWritesAndCleansOldFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "app-2026-05-01.log")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer, err := NewDailyWriter(dir, "app", 31)
	if err != nil {
		t.Fatal(err)
	}
	writer.now = func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.Local)
	}
	defer writer.Close()

	if _, err := writer.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	created := filepath.Join(dir, "app-2026-06-14.log")
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("expected current log file: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("expected old log removed, stat err=%v", err)
	}
}

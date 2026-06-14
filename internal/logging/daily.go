package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type DailyWriter struct {
	mu            sync.Mutex
	dir           string
	prefix        string
	retentionDays int
	now           func() time.Time
	currentDate   string
	file          *os.File
}

func NewDailyWriter(dir string, prefix string, retentionDays int) (*DailyWriter, error) {
	if retentionDays <= 0 {
		retentionDays = 31
	}
	writer := &DailyWriter{
		dir:           dir,
		prefix:        prefix,
		retentionDays: retentionDays,
		now:           time.Now,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return writer, nil
}

func (w *DailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateLocked(); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *DailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *DailyWriter) rotateLocked() error {
	date := w.now().Format("2006-01-02")
	if w.file != nil && w.currentDate == date {
		return nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	path := filepath.Join(w.dir, fmt.Sprintf("%s-%s.log", w.prefix, date))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = file
	w.currentDate = date
	w.cleanupLocked()
	return nil
}

func (w *DailyWriter) cleanupLocked() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := w.now().AddDate(0, 0, -w.retentionDays)
	filePrefix := w.prefix + "-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(name, filePrefix), ".log")
		date, err := time.ParseInLocation("2006-01-02", datePart, time.Local)
		if err != nil || !date.Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(w.dir, name))
	}
}

var _ io.WriteCloser = (*DailyWriter)(nil)

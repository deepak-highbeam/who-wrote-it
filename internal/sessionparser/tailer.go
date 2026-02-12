package sessionparser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// Tailer watches a single file for new lines appended at the end.
// It uses polling (check file size periodically) rather than fsnotify
// for individual files, which is more reliable for files being appended
// to by other processes.
type Tailer struct {
	path     string
	offset   int64
	interval time.Duration
}

// NewTailer creates a tailer that starts reading from the given offset.
// If offset is 0 and the file exists, it starts from the beginning.
// interval controls poll frequency (default: 500ms).
func NewTailer(path string, offset int64, interval time.Duration) *Tailer {
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	return &Tailer{
		path:     path,
		offset:   offset,
		interval: interval,
	}
}

// Tail opens the file, seeks to the stored offset, and sends new lines
// on the lines channel as they are appended. It blocks until ctx is
// cancelled, at which point it returns the final offset for persistence.
//
// If the file does not exist yet, Tail waits for it to appear.
// If the file is truncated (offset > file size), it resets to the beginning.
func (t *Tailer) Tail(ctx context.Context, lines chan<- []byte) (finalOffset int64, err error) {
	// Wait for the file to exist.
	for {
		if _, err := os.Stat(t.path); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return t.offset, nil
		case <-time.After(t.interval):
		}
	}

	f, err := os.Open(t.path)
	if err != nil {
		return t.offset, fmt.Errorf("open %s: %w", t.path, err)
	}
	defer f.Close()

	// Check for truncation.
	info, err := f.Stat()
	if err != nil {
		return t.offset, fmt.Errorf("stat %s: %w", t.path, err)
	}
	if info.Size() < t.offset {
		// file truncated, reset offset
		t.offset = 0
	}

	// Seek to the stored offset.
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return t.offset, fmt.Errorf("seek %s to %d: %w", t.path, t.offset, err)
	}

	reader := bufio.NewReader(f)
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		// Try to read all available lines.
		for {
			lineBytes, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					// Partial line (no newline yet) -- put bytes back for next read.
					// We only process complete lines.
					break
				}
				return t.offset, fmt.Errorf("read %s: %w", t.path, err)
			}

			// Remove trailing newline.
			if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
				lineBytes = lineBytes[:len(lineBytes)-1]
			}
			if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\r' {
				lineBytes = lineBytes[:len(lineBytes)-1]
			}

			t.offset += int64(len(lineBytes)) + 1 // +1 for the newline

			if len(lineBytes) == 0 {
				continue
			}

			// Make a copy so the caller owns the bytes.
			lineCopy := make([]byte, len(lineBytes))
			copy(lineCopy, lineBytes)

			select {
			case lines <- lineCopy:
			case <-ctx.Done():
				return t.offset, nil
			}
		}

		// Wait for more data.
		select {
		case <-ctx.Done():
			return t.offset, nil
		case <-ticker.C:
			// Re-stat to detect truncation or growth.
			info, err := os.Stat(t.path)
			if err != nil {
				// File removed -- wait for it to reappear.
				continue
			}
			if info.Size() < t.offset {
				t.offset = 0
				f.Close()
				f, err = os.Open(t.path)
				if err != nil {
					return t.offset, fmt.Errorf("reopen %s: %w", t.path, err)
				}
				reader = bufio.NewReader(f)
			}
		}
	}
}

// Offset returns the current read offset.
func (t *Tailer) Offset() int64 {
	return t.offset
}

// Path returns the file path being tailed.
func (t *Tailer) Path() string {
	return t.path
}

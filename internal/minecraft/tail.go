package minecraft

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// Tail starts at EOF so launching the watcher never replays historical requests.
func Tail(ctx context.Context, path string, interval time.Duration, handle func(string)) error {
	return tail(ctx, path, interval, handle, func() {})
}

func tail(ctx context.Context, path string, interval time.Duration, handle func(string), ready func()) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	ready()
	pending := ""
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		next, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !os.SameFile(info, next) {
			replacement, err := os.Open(path)
			if err != nil {
				return err
			}
			f.Close()
			f = replacement
			info = next
			offset = 0
			pending = ""
		} else if next.Size() < offset {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			offset = 0
			pending = ""
		}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			n, readErr := f.Read(buf)
			offset += int64(n)
			pending += string(buf[:n])
			for {
				line, rest, ok := strings.Cut(pending, "\n")
				if !ok {
					break
				}
				pending = rest
				if len(line) > 64*1024 {
					return errors.New("log line exceeds size limit")
				}
				handle(strings.TrimSuffix(line, "\r"))
			}
			if len(pending) > 64*1024 {
				return errors.New("log line exceeds size limit")
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
	}
}

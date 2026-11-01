package bot

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileJournal struct {
	file  *os.File
	seen  map[string]bool
	lock  string
	last  map[string]time.Time
	calls []time.Time
}

// One process owns a journal. Started jobs are never automatically replayed.
func OpenJournal(path string) (*FileJournal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock := path + ".lock"
	if err := os.Mkdir(lock, 0700); err != nil {
		return nil, errors.New("audit journal is locked; another watcher or stale lock exists")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		os.Remove(lock)
		return nil, err
	}
	j := &FileJournal{file: f, seen: make(map[string]bool), last: make(map[string]time.Time), lock: lock}
	info, err := f.Stat()
	if err != nil {
		j.Close()
		return nil, err
	}
	if info.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], info.Size()-1); err != nil || last[0] != '\n' {
			j.Close()
			return nil, errors.New("audit journal has an incomplete final record; recovery required")
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		j.Close()
		return nil, err
	}
	s := bufio.NewScanner(f)
	for s.Scan() {
		var r Record
		if json.Unmarshal(s.Bytes(), &r) != nil || r.ID == "" {
			j.Close()
			return nil, errors.New("audit journal is corrupt; recovery required")
		}
		if (r.State == "inference_started" || r.State == "accepted") && r.Time.IsZero() {
			j.Close()
			return nil, errors.New("journal reservation has no timestamp; recovery required")
		}
		j.seen[r.ID] = true
		j.remember(r)
	}
	if err := s.Err(); err != nil {
		j.Close()
		return nil, err
	}
	return j, nil
}
func (j *FileJournal) Seen(id string) bool               { return j.seen[id] }
func (j *FileJournal) LastRequest(uuid string) time.Time { return j.last[strings.ToLower(uuid)] }

func (j *FileJournal) remember(r Record) {
	if r.State == "accepted" || r.State == "started" {
		key := strings.ToLower(r.UUID)
		if r.Time.After(j.last[key]) {
			j.last[key] = r.Time
		}
	}
	if r.State == "inference_started" && !r.Time.Before(time.Now().Add(-time.Hour)) {
		j.calls = append(j.calls, r.Time)
	}
}

func (j *FileJournal) Append(r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	n, err := j.file.Write(b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return errors.New("short journal write")
	}
	if err := j.file.Sync(); err != nil {
		return err
	}
	j.seen[r.ID] = true
	j.remember(r)
	return nil
}
func (j *FileJournal) Close() error {
	err := j.file.Close()
	removeErr := os.Remove(j.lock)
	if err != nil {
		return err
	}
	return removeErr
}

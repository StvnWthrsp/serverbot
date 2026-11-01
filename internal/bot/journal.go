package bot

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileJournal struct {
	mu       sync.Mutex
	file     *os.File
	seen     map[string]bool
	lock     string
	last     map[string]time.Time
	calls    []time.Time
	sessions map[string]string
	states   map[string]string
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
	j := &FileJournal{file: f, seen: make(map[string]bool), last: make(map[string]time.Time), sessions: make(map[string]string), states: make(map[string]string), lock: lock}
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
func (j *FileJournal) Seen(id string) bool { j.mu.Lock(); defer j.mu.Unlock(); return j.seen[id] }
func (j *FileJournal) LastRequest(uuid string) time.Time {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.last[strings.ToLower(uuid)]
}
func (j *FileJournal) Session(uuid string) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.sessions[strings.ToLower(uuid)]
}
func (j *FileJournal) SessionState(uuid string) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.states[strings.ToLower(uuid)]
}
func (j *FileJournal) Sessions() map[string]string {
	j.mu.Lock()
	defer j.mu.Unlock()
	result := make(map[string]string, len(j.sessions))
	for uuid, id := range j.sessions {
		result[uuid] = id
	}
	return result
}

func (j *FileJournal) remember(r Record) {
	if r.SessionID != "" {
		j.sessions[strings.ToLower(r.UUID)] = r.SessionID
		j.states[strings.ToLower(r.UUID)] = r.State
	}
	if r.State == "agent_reset" {
		delete(j.sessions, strings.ToLower(r.UUID))
	}
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
	j.mu.Lock()
	defer j.mu.Unlock()
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

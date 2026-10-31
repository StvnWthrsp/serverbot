package minecraft

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailSkipsHistoryAndWaitsForCompleteLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.log")
	os.WriteFile(path, []byte("old\n"), 0600)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	lines := make(chan string, 10)
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- tail(ctx, path, 5*time.Millisecond, func(line string) { lines <- line }, func() { close(ready) })
	}()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("tailer did not open")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("part")
	f.Sync()
	select {
	case line := <-lines:
		t.Fatalf("early line: %q", line)
	case <-time.After(30 * time.Millisecond):
	}
	f.WriteString("ial\r\n")
	f.Sync()
	select {
	case line := <-lines:
		if line != "partial" {
			t.Fatalf("got %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no complete line")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("tailer did not stop")
	}
}

package rcon

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func fakeServer(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		handle(conn)
	}()
	t.Cleanup(func() { l.Close(); <-done })
	return l.Addr().String()
}

func TestCommandHandlesMultipartAndEmptyReplies(t *testing.T) {
	for _, response := range []string{"", "There are 0 players online"} {
		t.Run(response, func(t *testing.T) {
			address := fakeServer(t, func(conn net.Conn) {
				id, kind, password, err := readPacket(conn)
				if err != nil || id != 1 || kind != 3 || password != "test-only" {
					t.Error("invalid auth packet")
					return
				}
				writePacket(conn, 1, 0, "")
				writePacket(conn, 1, 2, "")
				id, kind, command, err := readPacket(conn)
				if err != nil || id != 2 || kind != 2 || command != "list" {
					t.Error("invalid command")
					return
				}
				if _, _, _, err := readPacket(conn); err != nil {
					t.Error(err)
					return
				}
				mid := len(response) / 2
				writePacket(conn, 2, 0, response[:mid])
				writePacket(conn, 2, 0, response[mid:])
				writePacket(conn, 3, 0, "")
			})
			got, err := (Client{Address: address, Password: "test-only"}).Command(t.Context(), "list")
			if err != nil || got != response {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestAuthenticationFailureDoesNotSendCommand(t *testing.T) {
	address := fakeServer(t, func(conn net.Conn) {
		readPacket(conn)
		writePacket(conn, -1, 2, "")
		var b [1]byte
		if n, _ := conn.Read(b[:]); n != 0 {
			t.Error("command sent after authentication rejection")
		}
	})
	_, err := (Client{Address: address, Password: "do-not-leak"}).Command(t.Context(), "list")
	if err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestCommandCancellationClosesConnection(t *testing.T) {
	address := fakeServer(t, func(conn net.Conn) { readPacket(conn); io.Copy(io.Discard, conn) })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (Client{Address: address, Password: "test-only"}).Command(ctx, "list")
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("cancellation was not respected: %v", err)
	}
}

func TestPacketBounds(t *testing.T) {
	for _, size := range []uint32{0, 9, maxPacket + 1, ^uint32(0)} {
		var b bytes.Buffer
		binary.Write(&b, binary.LittleEndian, size)
		if _, _, _, err := readPacket(&b); err == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
	var b bytes.Buffer
	writePacket(&b, 2, 0, "ok")
	data := b.Bytes()
	data[len(data)-1] = 1
	if _, _, _, err := readPacket(bytes.NewReader(data)); err == nil {
		t.Fatal("accepted invalid terminator")
	}
}

func FuzzReadPacket(f *testing.F) {
	var b bytes.Buffer
	writePacket(&b, 2, 0, "hello")
	f.Add(b.Bytes())
	f.Add([]byte{0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) { readPacket(bytes.NewReader(data)) })
}

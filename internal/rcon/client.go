package rcon

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const maxPacket = 1024 * 1024

type Client struct{ Address, Password string }

func (c Client) Command(ctx context.Context, command string) (string, error) {
	if c.Password == "" {
		return "", errors.New("RCON password is missing")
	}
	if len(command) > 4096 || strings.ContainsAny(command, "\x00\r\n") {
		return "", errors.New("invalid RCON command")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return "", errors.New("cannot connect to RCON")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", errors.New("cannot set RCON deadline")
	}
	if err := writePacket(conn, 1, 3, c.Password); err != nil {
		return "", errors.New("RCON authentication write failed")
	}
	id, kind, _, err := readPacket(conn)
	if err != nil {
		return "", errors.New("RCON authentication response failed")
	}
	if kind == 0 {
		id, kind, _, err = readPacket(conn)
	}
	if err != nil || id != 1 || kind != 2 {
		return "", errors.New("RCON authentication rejected")
	}
	if err := writePacket(conn, 2, 2, command); err != nil {
		return "", errors.New("RCON command write failed")
	}
	// A second command supplies a response boundary, including for an empty response.
	if err := writePacket(conn, 3, 2, ""); err != nil {
		return "", errors.New("RCON response boundary write failed")
	}
	var result strings.Builder
	for {
		id, kind, payload, err := readPacket(conn)
		if err != nil {
			return "", errors.New("RCON command response failed")
		}
		if kind != 0 {
			return "", errors.New("unexpected RCON response type")
		}
		if id == 3 {
			return result.String(), nil
		}
		if id != 2 || result.Len()+len(payload) > maxPacket {
			return "", errors.New("invalid RCON response")
		}
		result.WriteString(payload)
	}
}

func writePacket(w io.Writer, id, kind int32, payload string) error {
	buf := make([]byte, 14+len(payload))
	binary.LittleEndian.PutUint32(buf, uint32(len(buf)-4))
	binary.LittleEndian.PutUint32(buf[4:], uint32(id))
	binary.LittleEndian.PutUint32(buf[8:], uint32(kind))
	copy(buf[12:], payload)
	for len(buf) > 0 {
		n, err := w.Write(buf)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		buf = buf[n:]
	}
	return nil
}

func readPacket(r io.Reader) (int32, int32, string, error) {
	var size uint32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}
	if size < 10 || size > maxPacket {
		return 0, 0, "", fmt.Errorf("invalid packet size %d", size)
	}
	buf := make([]byte, int(size))
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, 0, "", err
	}
	if buf[len(buf)-1] != 0 || buf[len(buf)-2] != 0 {
		return 0, 0, "", errors.New("invalid RCON terminator")
	}
	return int32(binary.LittleEndian.Uint32(buf)), int32(binary.LittleEndian.Uint32(buf[4:])), string(buf[8 : len(buf)-2]), nil
}

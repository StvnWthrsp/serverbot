package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	ServerDir             string `json:"server_dir"`
	RCONAddress           string `json:"rcon_address"`
	AuditFile             string `json:"audit_file"`
	PollIntervalMS        int    `json:"poll_interval_ms"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 64*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, errors.New("config must contain one JSON object")
	}
	if c.ServerDir == "" || c.AuditFile == "" {
		return c, errors.New("server_dir and audit_file are required")
	}
	host, port, err := net.SplitHostPort(c.RCONAddress)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 || (host != "127.0.0.1" && host != "::1") {
		return c, errors.New("rcon_address must use a loopback IP and valid port")
	}
	if c.PollIntervalMS < 100 || c.PollIntervalMS > 60000 {
		return c, errors.New("poll_interval_ms must be between 100 and 60000")
	}
	if c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > 300 {
		return c, errors.New("request_timeout_seconds must be between 1 and 300")
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	if !filepath.IsAbs(c.ServerDir) {
		c.ServerDir = filepath.Join(base, c.ServerDir)
	}
	if !filepath.IsAbs(c.AuditFile) {
		c.AuditFile = filepath.Join(base, c.AuditFile)
	}
	return c, nil
}

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

	"github.com/StvnWthrsp/minecraft-server-agent/internal/opencode"
)

type Limits struct {
	PlayerCooldownSeconds int `json:"player_cooldown_seconds"`
	QueueCapacity         int `json:"queue_capacity"`
	InferenceCallsPerHour int `json:"inference_calls_per_hour"`
	AgentRunsPerHour      int `json:"agent_runs_per_hour,omitempty"`
}

type Config struct {
	Limits                *Limits          `json:"limits,omitempty"`
	ServerDir             string           `json:"server_dir"`
	RCONAddress           string           `json:"rcon_address"`
	AuditFile             string           `json:"audit_file"`
	PollIntervalMS        int              `json:"poll_interval_ms"`
	RequestTimeoutSeconds int              `json:"request_timeout_seconds"`
	Planner               string           `json:"planner,omitempty"`
	OpenCode              *opencode.Config `json:"opencode,omitempty"`
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
	if err := c.Validate(); err != nil {
		return c, err
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

func (c *Config) Validate() error {
	if c.ServerDir == "" || c.AuditFile == "" {
		return errors.New("server_dir and audit_file are required")
	}
	host, port, err := net.SplitHostPort(c.RCONAddress)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 || (host != "127.0.0.1" && host != "::1") {
		return errors.New("rcon_address must use a loopback IP and valid port")
	}
	if c.PollIntervalMS < 100 || c.PollIntervalMS > 60000 {
		return errors.New("poll_interval_ms must be between 100 and 60000")
	}
	if c.RequestTimeoutSeconds < 1 || c.RequestTimeoutSeconds > 3600 {
		return errors.New("request_timeout_seconds must be between 1 and 3600")
	}
	if c.Planner == "" {
		c.Planner = "commands"
	}
	if c.Limits == nil {
		c.Limits = &Limits{PlayerCooldownSeconds: 10, QueueCapacity: 8, InferenceCallsPerHour: 60}
	}
	if c.Limits.AgentRunsPerHour == 0 {
		c.Limits.AgentRunsPerHour = 60
	}
	if c.Limits.AgentRunsPerHour < 1 || c.Limits.AgentRunsPerHour > 10000 {
		return errors.New("agent_runs_per_hour must be between 1 and 10000")
	}
	if c.Limits.PlayerCooldownSeconds < 1 || c.Limits.PlayerCooldownSeconds > 3600 || c.Limits.QueueCapacity < 1 || c.Limits.QueueCapacity > 100 || c.Limits.InferenceCallsPerHour < 1 || c.Limits.InferenceCallsPerHour > 10000 {
		return errors.New("limits require player_cooldown_seconds=1..3600, queue_capacity=1..100 and inference_calls_per_hour=1..10000")
	}
	switch c.Planner {
	case "commands":
		if c.OpenCode != nil {
			return errors.New("opencode configuration requires planner=agent or planner=opencode")
		}
	case "opencode", "agent":
		if c.OpenCode == nil {
			return errors.New("opencode configuration is required")
		}
		if err := c.OpenCode.Validate(); err != nil {
			return err
		}
	default:
		return errors.New("planner must be agent, commands or opencode")
	}
	return nil
}

// SaveNew refuses to overwrite any existing file, including symlinks.
func SaveNew(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("cannot create configuration; choose a new path in an existing directory")
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

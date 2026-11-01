package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/config"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/opencode"
)

type questionnaire struct {
	input *bufio.Scanner
	out   io.Writer
	err   error
	ctx   context.Context
}

func (q *questionnaire) ask(label, fallback string) string {
	if q.err != nil {
		return ""
	}
	fmt.Fprintf(q.out, "%s [%s]: ", label, fallback)
	answer := make(chan *string, 1)
	go func() {
		if !q.input.Scan() {
			answer <- nil
			return
		}
		value := strings.TrimSpace(q.input.Text())
		answer <- &value
	}()
	select {
	case <-q.ctx.Done():
		q.err = q.ctx.Err()
		return ""
	case value := <-answer:
		if value == nil {
			q.err = errors.New("setup input ended; no configuration written")
			return ""
		}
		if *value == "" {
			return fallback
		}
		return *value
	}
}

func (q *questionnaire) number(label string, fallback int) int {
	value := q.ask(label, strconv.Itoa(fallback))
	n, err := strconv.Atoi(value)
	if err != nil && q.err == nil {
		q.err = fmt.Errorf("%s requires a whole number", label)
	}
	return n
}

func setup(ctx context.Context, input io.Reader, out io.Writer, path string) error {
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		return errors.New("configuration already exists or cannot be inspected; choose a new -config path")
	}
	fmt.Fprintln(out, "Minecraft Server Agent setup. Press Enter for defaults. No passwords or API keys are entered here.")
	q := questionnaire{input: bufio.NewScanner(input), out: out, ctx: ctx}
	c := config.Config{PollIntervalMS: 1000, RequestTimeoutSeconds: 45}
	c.ServerDir = q.ask("Minecraft server directory (relative to your current directory)", ".")
	c.RCONAddress = q.ask("RCON address (loopback only)", "127.0.0.1:25575")
	c.AuditFile = "data/audit.jsonl"
	c.Planner = strings.ToLower(q.ask("Planner: commands or opencode", "commands"))
	keyEnv := ""
	if c.Planner == "opencode" {
		fmt.Fprintf(out, "Install OpenCode %s and start it with: opencode serve --hostname 127.0.0.1 --port 4096\nUse a dedicated account/configuration. See docs/opencode.md for details.\n", opencode.Version)
		c.OpenCode = &opencode.Config{}
		c.OpenCode.URL = q.ask("OpenCode server URL", "http://127.0.0.1:4096")
		c.OpenCode.ProviderID = q.ask("Provider identifier", "openrouter")
		c.OpenCode.ModelID = q.ask("Model identifier (for OpenRouter include the publisher prefix)", "")
		keyEnv = q.ask("API-key environment variable (blank reuses an existing or OAuth connection)", "")
		fmt.Fprintln(out, "For OAuth, use opencode auth login --standalone under the same dedicated account/directories, then restart OpenCode. Set MSA_OPENCODE_PASSWORD to the local server password.")
	}
	c.Limits = &config.Limits{}
	c.Limits.PlayerCooldownSeconds = q.number("Per-player cooldown in seconds", 10)
	c.Limits.QueueCapacity = q.number("Maximum waiting requests", 8)
	c.Limits.InferenceCallsPerHour = q.number("Maximum inference attempts per rolling hour", 60)
	checks := strings.ToLower(q.ask("Check server files and connections after saving? yes or no", "yes"))
	if q.err != nil {
		return q.err
	}
	if checks != "yes" && checks != "no" {
		return errors.New("connection check choice must be yes or no; no configuration written")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	c.ServerDir, err = filepath.Abs(c.ServerDir)
	if err != nil {
		return err
	}
	if err := config.SaveNew(path, c); err != nil {
		return err
	}
	fmt.Fprintf(out, "Configuration saved to %s.\nSet MSA_RCON_PASSWORD in your process/service environment. Enable RCON and online-mode=true in Minecraft; keep RCON firewalled to local access.\n", path)
	if keyEnv != "" {
		if err := run(ctx, []string{"connect", "-config", path, "-key-env", keyEnv}, out); err != nil {
			return fmt.Errorf("configuration saved, but provider connection failed; set the key and MSA_OPENCODE_PASSWORD, then run connect -key-env %s: %w", keyEnv, err)
		}
		fmt.Fprintln(out, "You can now remove the API-key variable from this application's environment; OpenCode stores the credential.")
	}
	if checks == "yes" {
		if err := run(ctx, []string{"doctor", "-config", path}, out); err != nil {
			return fmt.Errorf("configuration saved; fix the diagnostics and rerun doctor: %w", err)
		}
	}
	fmt.Fprintf(out, "Next: minecraft-server-agent doctor -config %s\nThen: minecraft-server-agent run -config %s\n", path, path)
	if c.Planner == "opencode" {
		fmt.Fprintln(out, "Optional: doctor -check-model verifies inference with one budgeted provider call.")
	}
	return nil
}

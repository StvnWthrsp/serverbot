package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/StvnWthrsp/serverbot/internal/bot"
	"github.com/StvnWthrsp/serverbot/internal/config"
	"github.com/StvnWthrsp/serverbot/internal/minecraft"
	"github.com/StvnWthrsp/serverbot/internal/opencode"
	"github.com/StvnWthrsp/serverbot/internal/rcon"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("serverbot stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: serverbot version | doctor | run | request | plan | connect (use -h for command flags)")
	}
	if args[0] == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	if args[0] != "doctor" && args[0] != "run" && args[0] != "request" && args[0] != "plan" && args[0] != "connect" {
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	configPath := flags.String("config", "serverbot.json", "path to configuration JSON")
	player := flags.String("player", "", "OP player name (local diagnostic request only)")
	text := flags.String("message", "", "serverbot request (request or plan)")
	var keyEnv *string
	if args[0] == "connect" {
		keyEnv = flags.String("key-env", "SERVERBOT_PROVIDER_API_KEY", "environment variable containing the selected provider's API key")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	c, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	timeout := time.Duration(c.RequestTimeoutSeconds) * time.Second
	var planner bot.Planner = bot.CommandPlanner{}
	var model *opencode.Planner
	if c.Planner == "opencode" {
		model, err = opencode.New(*c.OpenCode, os.Getenv("SERVERBOT_OPENCODE_USERNAME"), os.Getenv("SERVERBOT_OPENCODE_PASSWORD"))
		if err != nil {
			return err
		}
		defer model.Close()
		planner = model
	}
	if args[0] == "connect" {
		if model == nil {
			return errors.New("connect requires planner opencode in the configuration")
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := model.ConnectKey(reqCtx, os.Getenv(*keyEnv)); err != nil {
			return err
		}
		fmt.Fprintln(out, "Provider credential saved by OpenCode. Use serverbot plan to check model access.")
		return nil
	}
	if args[0] == "plan" {
		if *text == "" {
			return errors.New("plan requires -message")
		}
		if len(*text) > 512 {
			return errors.New("planner input exceeds 512 bytes")
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		proposal, err := planner.Plan(reqCtx, *text)
		if err != nil {
			return err
		}
		if err := bot.ValidatePlan(proposal); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(proposal)
	}
	password := os.Getenv("SERVERBOT_RCON_PASSWORD")
	if password == "" {
		return errors.New("set SERVERBOT_RCON_PASSWORD in the process environment")
	}
	rc := rcon.Client{Address: c.RCONAddress, Password: password}
	if args[0] == "doctor" {
		for _, name := range []string{"server.properties", "ops.json", filepath.Join("logs", "latest.log")} {
			if _, err := os.Stat(filepath.Join(c.ServerDir, name)); err != nil {
				return fmt.Errorf("required server file unavailable: %s", name)
			}
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if _, err := rc.Command(reqCtx, "list"); err != nil {
			return err
		}
		if model != nil {
			if err := model.Check(reqCtx); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "Configuration and RCON connectivity OK. Chat authorization is checked for each request.")
		return nil
	}
	journal, err := bot.OpenJournal(c.AuditFile)
	if err != nil {
		return err
	}
	defer journal.Close()
	auth := minecraft.Authorizer{Dir: c.ServerDir}
	engine := bot.Engine{Auth: auth, Planner: planner, RCON: rc, Audit: journal}
	if args[0] == "request" {
		if *player == "" || *text == "" {
			return errors.New("request requires -player and -message")
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		result, err := engine.Handle(reqCtx, minecraft.Request{ID: rand.Text(), Player: *player, Text: *text})
		if err != nil {
			return err
		}
		fmt.Fprintln(out, result)
		return nil
	}
	slog.Info("watching Minecraft chat", "server_dir", c.ServerDir, "planner", c.Planner)
	return minecraft.Tail(ctx, filepath.Join(c.ServerDir, "logs", "latest.log"), time.Duration(c.PollIntervalMS)*time.Millisecond, func(line string) {
		req, ok := minecraft.ParseChat(line)
		if !ok {
			return
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		result, err := engine.Handle(reqCtx, req)
		if err != nil {
			slog.Warn("request refused or failed", "id", req.ID, "player", req.Player, "error", err)
			if _, authErr := auth.Authorize(req.Player); authErr != nil {
				return
			}
			result = "Request failed. Check the ServerBot process logs."
		}
		if result == "" {
			return
		}
		if len(result) > 512 {
			result = result[:512]
		}
		payload, _ := json.Marshal(map[string]string{"text": "[ServerBot] " + strings.ToValidUTF8(result, "?"), "color": "gold"})
		replyCtx, replyCancel := context.WithTimeout(ctx, timeout)
		defer replyCancel()
		if _, err := rc.Command(replyCtx, "tellraw "+req.Player+" "+string(payload)); err != nil {
			slog.Warn("chat reply failed", "id", req.ID, "error", err)
		}
	})
}

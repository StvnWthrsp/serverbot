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

	"github.com/StvnWthrsp/minecraft-server-agent/internal/bot"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/config"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/opencode"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/rcon"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("minecraft-server-agent stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: minecraft-server-agent setup | version | doctor | run | request | plan | connect (use -h for command flags)")
	}
	if args[0] == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	if args[0] != "setup" && args[0] != "doctor" && args[0] != "run" && args[0] != "request" && args[0] != "plan" && args[0] != "connect" {
		return errors.New("unknown command")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	configPath := flags.String("config", "minecraft-server-agent.json", "path to configuration JSON")
	player := flags.String("player", "", "OP player name (local diagnostic request only)")
	text := flags.String("message", "", "minecraft-server-agent request (request or plan)")
	var checkModel *bool
	if args[0] == "doctor" {
		checkModel = flags.Bool("check-model", false, "make one budgeted inference call to verify model access")
	}
	var keyEnv *string
	if args[0] == "connect" {
		keyEnv = flags.String("key-env", "MSA_PROVIDER_API_KEY", "environment variable containing the selected provider's API key")
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
	if args[0] == "setup" {
		return setup(ctx, os.Stdin, out, *configPath)
	}
	c, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("cannot load configuration; run minecraft-server-agent setup -config %s: %w", *configPath, err)
	}
	if args[0] == "doctor" {
		return doctor(ctx, c, out, *checkModel)
	}
	timeout := time.Duration(c.RequestTimeoutSeconds) * time.Second
	var planner bot.Planner = bot.CommandPlanner{}
	var model *opencode.Planner
	if c.Planner == "opencode" {
		model, err = opencode.New(*c.OpenCode, os.Getenv("MSA_OPENCODE_USERNAME"), os.Getenv("MSA_OPENCODE_PASSWORD"))
		if err != nil {
			return err
		}
		defer model.Close()
		planner = bot.BudgetPlanner{Planner: model, Path: c.AuditFile + ".inference", MaxCallsPerHour: c.Limits.InferenceCallsPerHour}
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
		fmt.Fprintln(out, "Provider credential saved by OpenCode. Use minecraft-server-agent plan to check model access.")
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
	password := os.Getenv("MSA_RCON_PASSWORD")
	if password == "" {
		return errors.New("set MSA_RCON_PASSWORD in the process environment")
	}
	rc := rcon.Client{Address: c.RCONAddress, Password: password}
	journal, err := bot.OpenJournal(c.AuditFile)
	if err != nil {
		return err
	}
	defer journal.Close()
	auth := minecraft.Authorizer{Dir: c.ServerDir}
	engine := bot.Engine{Auth: auth, Planner: planner, RCON: rc, Audit: journal, Cooldown: time.Duration(c.Limits.PlayerCooldownSeconds) * time.Second}
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
	workerCtx, stopWorker := context.WithCancel(ctx)
	queue := bot.NewQueue(c.Limits.QueueCapacity, timeout, auth, func(reqCtx context.Context, req minecraft.Request) {
		result, err := engine.Handle(reqCtx, req)
		if err != nil {
			slog.Warn("request refused or failed", "id", req.ID, "player", req.Player, "error", err)
			if _, authErr := auth.Authorize(req.Player); authErr != nil {
				return
			}
			result = "Request failed. Check the Minecraft Server Agent process logs."
			if errors.Is(err, bot.ErrCooldown) || errors.Is(err, bot.ErrInferenceLimit) {
				result = err.Error()
			}
		}
		if result == "" {
			return
		}
		if len(result) > 512 {
			result = result[:512]
		}
		payload, _ := json.Marshal(map[string]string{"text": "[Minecraft Server Agent] " + strings.ToValidUTF8(result, "?"), "color": "gold"})
		replyCtx, replyCancel := context.WithTimeout(workerCtx, timeout)
		defer replyCancel()
		if _, err := rc.Command(replyCtx, "tellraw "+req.Player+" "+string(payload)); err != nil {
			slog.Warn("chat reply failed", "id", req.ID, "error", err)
		}
	})
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); queue.Run(workerCtx) }()
	defer func() { stopWorker(); <-workerDone }()
	return minecraft.Tail(workerCtx, filepath.Join(c.ServerDir, "logs", "latest.log"), time.Duration(c.PollIntervalMS)*time.Millisecond, func(line string) {
		req, ok := minecraft.ParseChat(line)
		if !ok {
			return
		}
		if err := queue.Submit(workerCtx, req); err != nil {
			slog.Warn("request not queued", "player", req.Player, "error", err)
		}
	})
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/StvnWthrsp/minecraft-server-agent/internal/bot"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/config"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/minecraft"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/opencode"
	"github.com/StvnWthrsp/minecraft-server-agent/internal/rcon"
)

func doctor(ctx context.Context, c config.Config, out io.Writer, checkModel bool) error {
	failed := false
	check := func(label, help string, fn func(context.Context) error) {
		checkCtx, cancel := context.WithTimeout(ctx, time.Duration(c.RequestTimeoutSeconds)*time.Second)
		defer cancel()
		if err := fn(checkCtx); err != nil {
			failed = true
			fmt.Fprintf(out, "FAIL %s: %v. %s\n", label, err, help)
		} else {
			fmt.Fprintf(out, "OK %s\n", label)
		}
	}
	check("Minecraft files and authorization settings", "Check server_dir, server.properties, ops.json and logs/latest.log; start Minecraft once to create missing files.", func(context.Context) error {
		for _, name := range []string{"server.properties", "ops.json", filepath.Join("logs", "latest.log")} {
			info, err := os.Stat(filepath.Join(c.ServerDir, name))
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("required server file unavailable: %s", name)
			}
		}
		return (minecraft.Authorizer{Dir: c.ServerDir}).Check()
	})
	check("RCON", "Set MSA_RCON_PASSWORD, enable-rcon=true, verify rcon_address and keep the running server reachable locally.", func(ctx context.Context) error {
		password := os.Getenv("MSA_RCON_PASSWORD")
		if password == "" {
			return errors.New("MSA_RCON_PASSWORD is missing")
		}
		_, err := (rcon.Client{Address: c.RCONAddress, Password: password}).Command(ctx, "list")
		return err
	})
	if c.Planner == "opencode" || c.Planner == "agent" {
		check("OpenCode runtime", "Start the pinned runtime at opencode.url and set MSA_OPENCODE_PASSWORD to its startup password.", func(ctx context.Context) error {
			model, err := opencode.New(*c.OpenCode, os.Getenv("MSA_OPENCODE_USERNAME"), os.Getenv("MSA_OPENCODE_PASSWORD"))
			if err != nil {
				return err
			}
			defer model.Close()
			return model.Check(ctx)
		})
		if checkModel {
			check("Provider/model access (one inference attempt)", "Check provider_id/model_id and use connect for an API key, or OpenCode's OAuth sign-in. Check the inference ledger if the hourly cap is reached.", func(ctx context.Context) error {
				model, err := opencode.New(*c.OpenCode, os.Getenv("MSA_OPENCODE_USERNAME"), os.Getenv("MSA_OPENCODE_PASSWORD"))
				if err != nil {
					return err
				}
				defer model.Close()
				p := bot.BudgetPlanner{Planner: model, Path: c.AuditFile + ".inference", MaxCallsPerHour: c.Limits.InferenceCallsPerHour}
				proposal, err := p.Plan(ctx, "What is the server status?")
				if err != nil {
					return err
				}
				if proposal.Action != "status" {
					return errors.New("model did not return the expected status proposal")
				}
				return nil
			})
		} else {
			fmt.Fprintln(out, "SKIP Provider/model access: use doctor -check-model to make one budgeted inference call.")
		}
	}
	fmt.Fprintf(out, "Limits: %ds player cooldown, %d waiting requests, %d inference attempts per rolling hour.\n", c.Limits.PlayerCooldownSeconds, c.Limits.QueueCapacity, c.Limits.InferenceCallsPerHour)
	if c.Planner == "agent" {
		fmt.Fprintf(out, "Agent mode: full OpenCode tools; %d agent interactions per rolling hour, each potentially making multiple model calls.\n", c.Limits.AgentRunsPerHour)
	}
	if failed {
		return errors.New("diagnostics failed; follow the instructions above and rerun doctor")
	}
	fmt.Fprintln(out, "Diagnostics passed. Chat authorization is checked again for every request.")
	return nil
}

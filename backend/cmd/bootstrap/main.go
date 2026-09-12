package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/bootstrap"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/schema"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // laboratory time zones must not depend on the container image
)

const usage = "usage: bootstrap schema | bootstrap setup [-credentials-file PATH]"

func main() { os.Exit(run(os.Args[1:])) }

// run logs to stderr so stdout carries only the one-time administrator credentials.
func run(args []string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if len(args) == 0 || (args[0] != "schema" && args[0] != "setup") || (args[0] == "schema" && len(args) != 1) {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("configuration rejected", "error", err)
		return 1
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("invalid PostgreSQL configuration")
		return 1
	}
	defer pool.Close()
	if args[0] == "setup" {
		return setup(ctx, args[1:], pool, logger)
	}
	if err := schema.Install(ctx, pool); err != nil {
		logger.Error("schema installation failed; use an empty database with CREATE permission")
		return 1
	}
	logger.Info("schema installed")
	return 0
}

func setup(ctx context.Context, args []string, pool *pgxpool.Pool, logger *slog.Logger) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	credentialsPath := flags.String("credentials-file", "", "new file (mode 0600) that receives the generated administrator password")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	cfg, err := config.LoadSetup(os.Getenv)
	if err != nil {
		logger.Error("configuration rejected", "error", err)
		return 1
	}
	// Choose the password destination before writing anything, so the password
	// is neither lost nor captured by a log collector reading stdout.
	var out io.Writer = os.Stdout
	var file *os.File
	if *credentialsPath != "" {
		if file, err = os.OpenFile(*credentialsPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err != nil {
			logger.Error("cannot create credentials file; the path must not exist yet")
			return 1
		}
		out = file
	} else if info, statErr := os.Stdout.Stat(); statErr != nil || info.Mode()&os.ModeCharDevice == 0 {
		logger.Error("stdout is not a terminal; pass -credentials-file to receive the administrator password")
		return 1
	}
	result, err := bootstrap.Setup(ctx, pool, cfg)
	if err == nil && result.AdministratorCreated {
		_, err = fmt.Fprintf(out, "Administrator created. This password is shown only once; change it after signing in.\nemail: %s\npassword: %s\n", cfg.AdminEmail, result.AdminPassword)
	}
	if file != nil {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if !result.AdministratorCreated {
			_ = os.Remove(*credentialsPath)
		}
	}
	if err != nil {
		logger.Error("setup failed", "error", err)
		return 1
	}
	logger.Info("setup finished", "laboratoryCreated", result.LaboratoryCreated, "administratorCreated", result.AdministratorCreated)
	return 0
}

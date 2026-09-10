package main

import (
	"context"
	"github.com/HappyHackingSpace/Misko/backend/internal/bootstrap"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // laboratory time zones must not depend on the container image
)

func main() { os.Exit(run()) }
func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("configuration rejected", "error", err)
		return 1
	}
	auth, err := config.LoadAuth(os.Getenv)
	if err != nil {
		logger.Error("configuration rejected", "error", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := bootstrap.Run(ctx, cfg, auth, logger); err != nil {
		logger.Error("server stopped", "error", err)
		return 1
	}
	logger.Info("server stopped")
	return 0
}

package main

import (
	"context"
	"flag"
	"fmt"
	analysisapp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/bootstrap"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/schema"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // laboratory time zones must not depend on the container image
)

const usage = `usage: bootstrap migrate
       bootstrap setup [-credentials-file PATH]
       bootstrap register-worker -name NAME -model-version VERSION -paradigm KEY[:VERSION] [-paradigm ...] [-credentials-file PATH]`

func main() { os.Exit(run(os.Args[1:])) }

// run logs to stderr so stdout carries only the one-time credentials: the
// administrator password of setup and the worker token of register-worker.
func run(args []string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	commands := map[string]func(context.Context, []string, *pgxpool.Pool, *slog.Logger) int{
		"migrate": migrate, "setup": setup, "register-worker": registerWorker,
	}
	if len(args) == 0 || commands[args[0]] == nil || (args[0] == "migrate" && len(args) != 1) {
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
	return commands[args[0]](ctx, args[1:], pool, logger)
}

func migrate(ctx context.Context, _ []string, pool *pgxpool.Pool, logger *slog.Logger) int {
	applied, err := schema.Migrate(ctx, pool)
	if err != nil {
		logger.Error("migration failed; the database was left unchanged", "error", err)
		return 1
	}
	logger.Info("schema up to date", "version", schema.Latest(), "applied", applied)
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
	out, file, ok := credentials(*credentialsPath, "administrator password", logger)
	if !ok {
		return 1
	}
	result, err := bootstrap.Setup(ctx, pool, cfg)
	if err == nil && result.AdministratorCreated {
		_, err = fmt.Fprintf(out, "Administrator created. This password is shown only once; change it after signing in.\nemail: %s\npassword: %s\n", cfg.AdminEmail, result.AdminPassword)
	}
	finish(file, *credentialsPath, err == nil && result.AdministratorCreated, &err)
	if err != nil {
		logger.Error("setup failed", "error", err)
		return 1
	}
	logger.Info("setup finished", "laboratoryCreated", result.LaboratoryCreated, "administratorCreated", result.AdministratorCreated, "workerSeeded", result.WorkerSeeded)
	return 0
}

// registerWorker registers an analysis worker directly in the database and,
// like setup with the administrator password, prints its token exactly once.
func registerWorker(ctx context.Context, args []string, pool *pgxpool.Pool, logger *slog.Logger) int {
	flags := flag.NewFlagSet("register-worker", flag.ContinueOnError)
	name := flags.String("name", "", "worker name, unique in the laboratory")
	model := flags.String("model-version", "", `model the worker runs, for example "misko-tracker-bgsub 2.2.0"`)
	var capabilities capabilityList
	flags.Var(&capabilities, "paradigm", "paradigm version the worker analyses, as KEY or KEY:VERSION; repeat for each paradigm")
	credentialsPath := flags.String("credentials-file", "", "new file (mode 0600) that receives the worker token")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*name) == "" || strings.TrimSpace(*model) == "" || len(capabilities) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	out, file, ok := credentials(*credentialsPath, "worker token", logger)
	if !ok {
		return 1
	}
	registered, err := bootstrap.RegisterWorker(ctx, pool, analysisapp.WorkerInput{Name: *name, ModelVersion: *model, Capabilities: capabilities})
	if err == nil {
		_, err = fmt.Fprintf(out, "Worker %s created. This token is shown only once; disabling the worker revokes it.\nid: %s\ntoken: %s\n", registered.Worker.Name, registered.Worker.ID, registered.Token)
	}
	finish(file, *credentialsPath, err == nil, &err)
	if err != nil {
		logger.Error("worker registration failed", "error", err)
		return 1
	}
	logger.Info("worker registered", "id", registered.Worker.ID, "name", registered.Worker.Name)
	return 0
}

// credentials chooses the destination of a one-time secret before anything is
// written, so the secret is neither lost nor captured by a log collector
// reading stdout: a new file with mode 0600, or stdout when it is a terminal.
func credentials(path, secret string, logger *slog.Logger) (io.Writer, *os.File, bool) {
	if path != "" {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			logger.Error("cannot create credentials file; the path must not exist yet")
			return nil, nil, false
		}
		return file, file, true
	}
	if info, err := os.Stdout.Stat(); err != nil || info.Mode()&os.ModeCharDevice == 0 {
		logger.Error("stdout is not a terminal; pass -credentials-file to receive the " + secret)
		return nil, nil, false
	}
	return os.Stdout, nil, true
}

// finish closes the credentials file and removes it when nothing was written.
func finish(file *os.File, path string, written bool, err *error) {
	if file == nil {
		return
	}
	if closeErr := file.Close(); *err == nil {
		*err = closeErr
	}
	if !written {
		_ = os.Remove(path)
	}
}

// capabilityList collects repeated -paradigm flags.
type capabilityList []domain.Capability

func (c *capabilityList) String() string {
	parts := make([]string, len(*c))
	for i, capability := range *c {
		parts[i] = fmt.Sprintf("%s:%d", capability.ParadigmKey, capability.ParadigmVersion)
	}
	return strings.Join(parts, ",")
}

func (c *capabilityList) Set(raw string) error {
	capability, err := bootstrap.ParseCapability(raw)
	if err != nil {
		return err
	}
	*c = append(*c, capability)
	return nil
}

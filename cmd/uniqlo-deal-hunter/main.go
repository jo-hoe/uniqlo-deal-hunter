// Command uniqlo-deal-hunter runs one scrape+notify pass over the Uniqlo DE
// sale catalogue and exits. Intended to be invoked by a Kubernetes CronJob.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jo-hoe/uniqlo-deal-hunter/internal/app"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/config"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/filter"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/logging"
	smtpn "github.com/jo-hoe/uniqlo-deal-hunter/internal/notifier/smtp"
	"github.com/jo-hoe/uniqlo-deal-hunter/internal/source/uniqlo"
	sqlitestore "github.com/jo-hoe/uniqlo-deal-hunter/internal/store/sqlite"
)

// defaultConfigPath is used when neither --config nor CONFIG_PATH is set.
const defaultConfigPath = "/run/config/config.yaml"

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml (overrides CONFIG_PATH)")
	flag.Parse()

	path := pickConfigPath(*cfgPath)
	if err := run(path); err != nil {
		// slog might not yet be configured; use plain stderr to be safe.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run wires everything and executes one pipeline pass.
func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	logger := logging.New(effectiveLogLevel(cfg.Logging.Level))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runner, cleanup, err := assemble(cfg, logger)
	if err != nil {
		return err
	}
	defer cleanup()

	return runner.Run(ctx)
}

// assemble constructs every collaborator. Returns a cleanup func the caller
// should always defer, whether Run succeeds or not.
func assemble(cfg *config.Config, logger *slog.Logger) (*app.Runner, func(), error) {
	src := uniqlo.NewClient(cfg.Source, logger)
	eval := filter.New(cfg.Rules)

	notif, err := smtpn.New(cfg.Notifier.SMTP)
	if err != nil {
		return nil, func() {}, err
	}
	st, err := sqlitestore.Open(cfg.Store.Path)
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = st.Close() }

	runner := app.NewRunner(cfg, src, eval, notif, st, logger)
	return runner, cleanup, nil
}

// pickConfigPath resolves the config path from --config, CONFIG_PATH, or
// the default.
func pickConfigPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("CONFIG_PATH"); env != "" {
		return env
	}
	return defaultConfigPath
}

// effectiveLogLevel gives the LOG_LEVEL env var precedence over the config,
// matching the reference project.
func effectiveLogLevel(cfgLevel string) string {
	if env := os.Getenv("LOG_LEVEL"); env != "" {
		return env
	}
	return cfgLevel
}

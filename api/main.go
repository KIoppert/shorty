package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: logKeys})
	slog.SetDefault(slog.New(handler).With("service.name", "shorty-api"))

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func logKeys(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.TimeKey:
		a.Key = "timestamp"
	case slog.LevelKey:
		a.Value = slog.StringValue(strings.ToLower(a.Value.String()))
	case slog.MessageKey:
		a.Key = "message"
	}
	return a
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "serve":
		return serve(ctx, cfg, db)
	case "migrate":
		return migrate(ctx, db)
	case "promote":
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: shorty promote <email>")
		}
		return promote(ctx, db, os.Args[2])
	default:
		return fmt.Errorf("unknown command %q, use serve, migrate or promote", command)
	}
}

func serve(ctx context.Context, cfg Config, db *pgxpool.Pool) error {
	app := &App{cfg: cfg, store: &Store{db: db}}
	servers := []*http.Server{
		{Addr: ":" + cfg.Port, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second},
		{Addr: ":" + cfg.MetricsPort, Handler: metricsHandler(), ReadHeaderTimeout: 5 * time.Second},
	}

	errs := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			slog.Info("listening", "addr", srv.Addr)
			errs <- srv.ListenAndServe()
		}()
	}

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}

func promote(ctx context.Context, db *pgxpool.Pool, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := (&Store{db: db}).PromoteUser(ctx, email); err != nil {
		return fmt.Errorf("promote %s: %w", email, err)
	}
	slog.Info("user promoted to admin", "email", email)
	return nil
}

package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/neko233-com/pay233-server/internal/admin"
	"github.com/neko233-com/pay233-server/internal/config"
	"github.com/neko233-com/pay233-server/internal/httpapi"
	"github.com/neko233-com/pay233-server/internal/logging"
	"github.com/neko233-com/pay233-server/internal/payment"
)

func main() {
	configPath := flag.String("config", "config.example.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	appLog, paymentLog, closeLogs, err := setupLoggers(cfg)
	if err != nil {
		slog.Error("setup logging", "error", err)
		os.Exit(1)
	}
	defer closeLogs()
	slog.SetDefault(appLog)

	registry := payment.NewRegistry()
	if err := payment.RegisterConfiguredProviders(registry, cfg.Channels); err != nil {
		slog.Error("register providers", "error", err)
		os.Exit(1)
	}

	store, err := payment.NewFileStore(cfg.Storage.PaymentsPath)
	if err != nil {
		slog.Error("open payment store", "path", cfg.Storage.PaymentsPath, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Error("close payment store", "error", err)
		}
	}()
	userStore, err := admin.NewUserStore(cfg.Storage.AdminUsersPath, cfg.Admin.Username, cfg.Admin.Password)
	if err != nil {
		slog.Error("open admin user store", "path", cfg.Storage.AdminUsersPath, "error", err)
		os.Exit(1)
	}
	auditStore, err := admin.NewAuditStore(cfg.Storage.AuditPath, cfg.Storage.AuditRetentionDays)
	if err != nil {
		slog.Error("open audit store", "path", cfg.Storage.AuditPath, "error", err)
		os.Exit(1)
	}
	if removed, err := auditStore.PruneExpired(time.Now().UTC()); err != nil {
		slog.Error("prune audit log", "error", err)
		os.Exit(1)
	} else if removed > 0 {
		slog.Info("pruned expired audit logs", "removed", removed)
	}
	handler := httpapi.NewServer(httpapi.Dependencies{
		Config:        cfg,
		Registry:      registry,
		Store:         store,
		UserStore:     userStore,
		AuditStore:    auditStore,
		Logger:        appLog,
		PaymentLogger: paymentLog,
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		slog.Info("pay233 server listening", "addr", cfg.HTTP.Addr)
		errs <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case sig := <-stop:
		slog.Info("shutdown requested", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown", "error", err)
			os.Exit(1)
		}
	}
}

func setupLoggers(cfg config.Config) (*slog.Logger, *slog.Logger, func(), error) {
	retention := cfg.Logging.RetentionDays
	appWriter, err := logging.NewDailyWriter(cfg.Logging.Dir, "app", retention)
	if err != nil {
		return nil, nil, nil, err
	}
	paymentWriter, err := logging.NewDailyWriter(filepath.Join(cfg.Logging.Dir, "payments"), "payment", retention)
	if err != nil {
		_ = appWriter.Close()
		return nil, nil, nil, err
	}

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	appLogger := slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, appWriter), opts))
	paymentLogger := slog.New(slog.NewJSONHandler(paymentWriter, opts))
	closeLogs := func() {
		_ = appWriter.Close()
		_ = paymentWriter.Close()
	}
	return appLogger, paymentLogger, closeLogs, nil
}

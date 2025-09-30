package main

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/app/bootstrap"
	"lucipedia/app/internal/platform/config"
	applog "lucipedia/app/internal/platform/log"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return eris.Wrap(err, "failure loading configuration")
	}

	logger, err := applog.NewLogger(cfg.LogLevel)
	if err != nil {
		return eris.Wrap(err, "failure initialising logger")
	}

	sentryHub, flush, err := applog.InitSentry(logger, applog.SentrySettings{
		DSN:         cfg.SentryDSN,
		Environment: cfg.Environment,
	})
	if err != nil {
		return eris.Wrap(err, "failure initialising sentry")
	}
	defer flush()

	result, err := bootstrap.Build(ctx, bootstrap.Dependencies{
		Config:    *cfg,
		Logger:    logger,
		SentryHub: sentryHub,
	})
	if err != nil {
		return eris.Wrap(err, "building application components")
	}

	defer func() {
		if result.Cleanup == nil {
			return
		}
		if closeErr := result.Cleanup(); closeErr != nil {
			logger.WithError(closeErr).Error("closing application resources")
		}
	}()

	transport := result.HTTPServer

	httpServer := &stdhttp.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", cfg.ServerPort),
		Handler: transport.Handler(),
	}

	logger.WithFields(logrus.Fields{
		"addr": httpServer.Addr,
	}).Info("starting http server")

	serverErrCh := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			serverErrCh <- err
		} else {
			serverErrCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErrCh:
		if err != nil {
			return eris.Wrap(err, "http server error")
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return eris.Wrap(err, "shutting down http server")
	}

	logger.Info("http server shut down cleanly")
	return nil
}

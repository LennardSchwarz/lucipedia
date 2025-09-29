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

	"lucipedia/app/internal/config"
	appdb "lucipedia/app/internal/db"
	apphttp "lucipedia/app/internal/http"
	"lucipedia/app/internal/llm"
	applog "lucipedia/app/internal/log"
	"lucipedia/app/internal/wiki"
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

	dbConn, err := appdb.Open(appdb.Options{Path: cfg.DBPath})
	if err != nil {
		return eris.Wrap(err, "opening database")
	}
	defer func() {
		if closeErr := appdb.Close(dbConn); closeErr != nil {
			logger.WithError(closeErr).Error("closing database")
		}
	}()

	if err := wiki.Migrate(ctx, dbConn, logger); err != nil {
		return eris.Wrap(err, "running migrations")
	}

	repository, err := wiki.NewRepository(dbConn, logger)
	if err != nil {
		return eris.Wrap(err, "building wiki repository")
	}

	client, err := llm.NewClient(llm.ClientOptions{
		APIKey:  cfg.LLMAPIKey,
		BaseURL: cfg.LLMEndpoint,
		Logger:  logger,
	})
	if err != nil {
		return eris.Wrap(err, "creating llm client")
	}

	if len(cfg.LLMModels) == 0 {
		return eris.New("LLM_MODELS must include at least one model name")
	}

	generatorModel := cfg.LLMModels[0]
	searcherModel := generatorModel
	if len(cfg.LLMModels) > 1 {
		searcherModel = cfg.LLMModels[1]
	}

	generator, err := llm.NewGenerator(llm.GeneratorOptions{
		Client: client,
		Model:  generatorModel,
	})
	if err != nil {
		return eris.Wrap(err, "initialising generator")
	}

	searcher, err := llm.NewSearcher(llm.SearcherOptions{
		Client: client,
		Model:  searcherModel,
	})
	if err != nil {
		return eris.Wrap(err, "initialising searcher")
	}

	wikiService, err := wiki.NewService(repository, generator, searcher, logger, sentryHub)
	if err != nil {
		return eris.Wrap(err, "creating wiki service")
	}

	transport, err := apphttp.NewServer(apphttp.Options{
		WikiService: wikiService,
		Repository:  repository,
		Generator:   generator,
		Database:    dbConn,
		Logger:      logger,
		SentryHub:   sentryHub,
	})
	if err != nil {
		return eris.Wrap(err, "initialising http transport")
	}

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

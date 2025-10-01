package bootstrap

import (
	"context"

	"github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"lucipedia/app/internal/data/database"
	"lucipedia/app/internal/data/migrations"
	datawiki "lucipedia/app/internal/data/wiki"
	domainwiki "lucipedia/app/internal/domain/wiki"
	"lucipedia/app/internal/infrastructure/llm/openai"
	"lucipedia/app/internal/platform/config"
	presentationhttp "lucipedia/app/internal/presentation/http"
)

type Dependencies struct {
	Config    config.Config
	Logger    *logrus.Logger
	SentryHub *sentry.Hub
}

type Result struct {
	WikiService domainwiki.Service
	HTTPServer  *presentationhttp.Server
	Database    *gorm.DB
	Cleanup     func() error
}

// Build composes the Lucipedia application layers and returns the constructed components.
func Build(ctx context.Context, deps Dependencies) (Result, error) {
	db, err := database.Open(database.Options{Path: deps.Config.DBPath})
	if err != nil {
		return Result{}, eris.Wrap(err, "opening database")
	}

	closeOnError := func(wrapper error) (Result, error) {
		if closeErr := database.Close(db); closeErr != nil && deps.Logger != nil {
			deps.Logger.WithError(closeErr).Error("closing database after bootstrap failure")
		}
		return Result{}, wrapper
	}

	if err := migrations.MigrateWiki(ctx, db, deps.Logger); err != nil {
		return closeOnError(eris.Wrap(err, "running wiki migrations"))
	}

	repo, err := datawiki.NewRepository(db, deps.Logger)
	if err != nil {
		return closeOnError(eris.Wrap(err, "creating wiki repository"))
	}

	if len(deps.Config.LLMModels) == 0 {
		return closeOnError(eris.New("LLM_MODELS must include at least one model name"))
	}

	client, err := openai.NewClient(openai.ClientOptions{
		APIKey:  deps.Config.LLMAPIKey,
		BaseURL: deps.Config.LLMEndpoint,
		Logger:  deps.Logger,
	})
	if err != nil {
		return closeOnError(eris.Wrap(err, "creating llm client"))
	}

	generatorModel := deps.Config.LLMModels[0]
	searcherModel := generatorModel
	if len(deps.Config.LLMModels) > 1 {
		searcherModel = deps.Config.LLMModels[1]
	}

	generator, err := openai.NewGenerator(openai.GeneratorOptions{
		Client: client,
		Model:  generatorModel,
	})
	if err != nil {
		return closeOnError(eris.Wrap(err, "initialising llm generator"))
	}

	searcher, err := openai.NewSearcher(openai.SearcherOptions{
		Client: client,
		Model:  searcherModel,
	})
	if err != nil {
		return closeOnError(eris.Wrap(err, "initialising llm searcher"))
	}

	wikiService, err := domainwiki.NewService(repo, generator, searcher, deps.Logger, deps.SentryHub)
	if err != nil {
		return closeOnError(eris.Wrap(err, "creating wiki service"))
	}

	httpServer, err := presentationhttp.NewServer(presentationhttp.Options{
		WikiService: wikiService,
		Logger:      deps.Logger,
		SentryHub:   deps.SentryHub,
		RateLimiter: presentationhttp.RateLimiterSettings{
			Burst:             deps.Config.RateLimit.Burst,
			RequestsPerSecond: deps.Config.RateLimit.RequestsPerSecond,
			ClientTTL:         deps.Config.RateLimit.ClientTTL,
		},
	})
	if err != nil {
		return closeOnError(eris.Wrap(err, "initialising http server"))
	}

	cleanup := func() error {
		return database.Close(db)
	}

	return Result{
		WikiService: wikiService,
		HTTPServer:  httpServer,
		Database:    db,
		Cleanup:     cleanup,
	}, nil
}

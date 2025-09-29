package http

import (
	stdhttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"lucipedia/app/internal/llm"
	"lucipedia/app/internal/wiki"
)

// Options configures the HTTP server wiring.
type Options struct {
	WikiService wiki.Service
	Repository  wiki.Repository
	Generator   llm.Generator
	Database    *gorm.DB
	Logger      *logrus.Logger
	SentryHub   *sentry.Hub
	RateLimiter RateLimiterSettings
}

// RateLimiterSettings configures the HTTP rate limiter behaviour.
type RateLimiterSettings struct {
	RequestsPerSecond float64
	Burst             int
	ClientTTL         time.Duration
}

// Server wires the HTTP transport layer via Huma and templ components.
type Server struct {
	api         huma.API
	mux         *stdhttp.ServeMux
	wiki        wiki.Service
	repository  wiki.Repository
	generator   llm.Generator
	logger      *logrus.Logger
	sentry      *sentry.Hub
	db          *gorm.DB
	rateLimiter *RateLimiter
}

// NewServer constructs the HTTP server.
func NewServer(opts Options) (*Server, error) {
	if opts.WikiService == nil {
		return nil, eris.New("wiki service is required")
	}
	if opts.Repository == nil {
		return nil, eris.New("wiki repository is required")
	}
	if opts.Generator == nil {
		return nil, eris.New("generator is required")
	}
	if opts.Database == nil {
		return nil, eris.New("database is required")
	}

	mux := stdhttp.NewServeMux()
	config := huma.DefaultConfig("Lucipedia", "1.0.0")

	api := humago.New(mux, config)

	srv := &Server{
		api:        api,
		mux:        mux,
		wiki:       opts.WikiService,
		repository: opts.Repository,
		generator:  opts.Generator,
		logger:     opts.Logger,
		sentry:     opts.SentryHub,
		db:         opts.Database,
	}

	settings := opts.RateLimiter
	if settings.Burst <= 0 {
		return nil, eris.New("rate limiter burst must be greater than zero")
	}
	if settings.RequestsPerSecond <= 0 {
		return nil, eris.New("rate limiter requests per second must be greater than zero")
	}
	if settings.ClientTTL <= 0 {
		return nil, eris.New("rate limiter client TTL must be greater than zero")
	}

	srv.rateLimiter = NewRateLimiter(settings.Burst, settings.RequestsPerSecond, settings.ClientTTL)

	srv.registerMiddlewares()
	srv.registerRoutes()

	return srv, nil
}

// Handler exposes the underlying HTTP handler for wiring into the application.
func (s *Server) Handler() stdhttp.Handler {
	return s.mux
}

// API exposes the underlying Huma API instance.
func (s *Server) API() huma.API {
	return s.api
}

func (s *Server) registerMiddlewares() {
	s.api.UseMiddleware(
		s.sentryMiddleware(),
		s.recoveryMiddleware(),
		s.requestIDMiddleware(),
		s.rateLimitMiddleware(),
		s.loggingMiddleware(),
	)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /favicon.ico", faviconHandler)
	s.mux.HandleFunc("HEAD /favicon.ico", faviconHandler)

	s.registerHomeRoute()
	s.registerRandomRoute()
	s.registerMostRecentRoute()
	s.registerWikiRoute()
	s.registerSearchRoute()
	s.registerHealthRoute()
}

func (s *Server) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	s.mux.ServeHTTP(w, r)
}

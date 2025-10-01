package http

import (
	stdhttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/domain/wiki"
)

// Options configures the HTTP server wiring.
type Options struct {
	WikiService wiki.Service
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
	logger      *logrus.Logger
	sentry      *sentry.Hub
	rateLimiter *RateLimiter
}

// NewServer constructs the HTTP server.
func NewServer(opts Options) (*Server, error) {
	if opts.WikiService == nil {
		return nil, eris.New("wiki service is required")
	}

	mux := stdhttp.NewServeMux()
	config := huma.DefaultConfig("Lucipedia", "1.0.0")

	api := humago.New(mux, config)

	srv := &Server{
		api:    api,
		mux:    mux,
		wiki:   opts.WikiService,
		logger: opts.Logger,
		sentry: opts.SentryHub,
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

	s.registerStaticRoute()

	s.registerHomeRoute()
	s.registerAllPagesRoute()
	s.registerRandomRoute()
	s.registerMostRecentRoute()
	s.registerWikiRoute()
	s.registerSearchRoute()
	s.registerHealthRoute()
}

func (s *Server) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	s.mux.ServeHTTP(w, r)
}

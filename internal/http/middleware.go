package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func (s *Server) requestIDMiddleware() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		reqID := uuid.NewString()
		goCtx := context.WithValue(ctx.Context(), requestIDContextKey, reqID)
		ctx = huma.WithContext(ctx, goCtx)
		ctx.SetHeader("X-Request-ID", reqID)

		if hub := sentry.GetHubFromContext(goCtx); hub != nil {
			hub.Scope().SetTag("request_id", reqID)
		}

		next(ctx)
	}
}

func (s *Server) loggingMiddleware() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if s.logger == nil {
			next(ctx)
			return
		}

		start := time.Now()
		next(ctx)

		status := ctx.Status()
		if status == 0 {
			status = stdhttp.StatusOK
		}

		fields := logrus.Fields{
			"method":      ctx.Method(),
			"status":      status,
			"duration_ms": float64(time.Since(start).Microseconds()) / 1000,
		}

		if op := ctx.Operation(); op != nil {
			fields["route"] = op.Path
		}

		if req, _ := humago.Unwrap(ctx); req != nil {
			fields["path"] = req.URL.Path
			fields["remote_addr"] = req.RemoteAddr
		}

		if requestID := RequestIDFromContext(ctx.Context()); requestID != "" {
			fields["request_id"] = requestID
		}

		entry := s.logger.WithFields(fields)
		if status >= 500 {
			entry.Error("request failed")
		} else {
			entry.Info("request completed")
		}
	}
}

func (s *Server) recoveryMiddleware() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		defer func() {
			if rec := recover(); rec != nil {
				var err error
				switch v := rec.(type) {
				case error:
					err = v
				default:
					err = fmt.Errorf("panic: %v", v)
				}

				s.recordError(ctx.Context(), err, "panic recovered", nil)

				if hub := sentry.GetHubFromContext(ctx.Context()); hub != nil {
					hub.RecoverWithContext(ctx.Context(), rec)
					hub.Flush(2 * time.Second)
				}

				ctx.SetHeader("Content-Type", "text/plain; charset=utf-8")
				ctx.SetStatus(stdhttp.StatusInternalServerError)
				_, _ = ctx.BodyWriter().Write([]byte("internal server error"))
			}
		}()

		next(ctx)
	}
}

func (s *Server) sentryMiddleware() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if s.sentry == nil {
			next(ctx)
			return
		}

		hub := s.sentry.Clone()
		scope := hub.Scope()
		scope.SetTag("http.method", ctx.Method())
		if op := ctx.Operation(); op != nil {
			scope.SetTag("http.route", op.Path)
		}

		goCtx := sentry.SetHubOnContext(ctx.Context(), hub)
		ctx = huma.WithContext(ctx, goCtx)

		defer hub.Flush(2 * time.Second)

		next(ctx)
	}
}

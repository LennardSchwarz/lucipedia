package http

import "context"

type contextKey string

const requestIDContextKey contextKey = "lucipedia/request-id"

// RequestIDFromContext extracts the request identifier from the context when available.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(requestIDContextKey).(string); ok {
		return value
	}
	return ""
}

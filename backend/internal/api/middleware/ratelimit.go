package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// RateLimit returns a per-IP rate limiting middleware.
// If rpm <= 0 the middleware is a no-op (rate limiting disabled).
func RateLimit(rpm int) func(http.Handler) http.Handler {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return httprate.LimitByRealIP(rpm, time.Minute)
}

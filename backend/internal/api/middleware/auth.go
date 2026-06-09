package middleware

import (
	"net/http"
	"strings"
)

// Auth returns middleware that enforces a static API key check.
// If apiKey is empty the middleware is a no-op (auth disabled).
// Clients pass the key via X-API-Key header or Authorization: Bearer <key>.
func Auth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				auth := r.Header.Get("Authorization")
				key = strings.TrimPrefix(auth, "Bearer ")
			}
			if key != apiKey {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

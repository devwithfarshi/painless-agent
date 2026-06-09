package middleware

import (
	"net/http"
	"strings"
)

// CORS returns permissive CORS middleware. origins is a comma-separated list
// of allowed origins; "*" allows all.
func CORS(origins string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	wildcard := false
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o == "*" {
			wildcard = true
			break
		}
		if o != "" {
			allowed[o] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if wildcard || (origin != "" && allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", func() string {
					if wildcard {
						return "*"
					}
					return origin
				}())
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

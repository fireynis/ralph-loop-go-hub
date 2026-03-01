package server

import (
	"net/http"
	"strings"

	"github.com/fireynis/ralph-hub/internal/config"
)

// corsMiddleware adds CORS headers to responses when the request's Origin
// matches one of the allowed origins. If the origins list is empty, all
// origins are allowed (wildcard). Preflight OPTIONS requests receive a 204.
func corsMiddleware(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if len(origins) == 0 {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		if origin != "" && (len(origins) == 0 || allowed[origin]) {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware checks the Authorization header for a valid Bearer token.
// If no API keys are configured, all requests are allowed (open mode).
func authMiddleware(keys []config.APIKey, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(keys) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		for _, k := range keys {
			if k.Key == token {
				next.ServeHTTP(w, r)
				return
			}
		}

		writeError(w, http.StatusUnauthorized, "unauthorized")
	})
}

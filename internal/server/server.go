package server

import (
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
)

// Server ties together the store, WebSocket hub, and webhook dispatcher
// behind an HTTP API.
type Server struct {
	store      store.Store
	hub        *ws.Hub
	dispatcher *webhook.Dispatcher
	config     config.Config
	upgrader   websocket.Upgrader
	frontendFS fs.FS // embedded frontend files, may be nil
}

// New creates a Server with all dependencies wired in.
func New(cfg config.Config, st store.Store, h *ws.Hub, d *webhook.Dispatcher) *Server {
	return &Server{
		store:      st,
		hub:        h,
		dispatcher: d,
		config:     cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// SetFrontendFS sets the embedded frontend filesystem for serving the SPA.
func (s *Server) SetFrontendFS(fsys fs.FS) {
	s.frontendFS = fsys
}

// Handler returns the fully wired http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health check (no auth).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Event ingestion (auth required).
	mux.Handle("POST /api/v1/events",
		authMiddleware(s.config.Auth.APIKeys, http.HandlerFunc(s.handlePostEvent)))

	// Read-only endpoints (no auth, for dashboard queries).
	mux.HandleFunc("GET /api/v1/instances", s.handleGetInstances)
	mux.HandleFunc("GET /api/v1/instances/{id}/history", s.handleGetInstanceHistory)
	mux.HandleFunc("GET /api/v1/sessions", s.handleGetSessions)
	mux.HandleFunc("GET /api/v1/sessions/{id}", s.handleGetSessionDetail)
	mux.HandleFunc("POST /api/v1/sessions/{id}/end", s.handleEndSession)
	mux.HandleFunc("GET /api/v1/stats", s.handleGetStats)

	// WebSocket endpoint (no auth).
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)

	if s.frontendFS != nil {
		mux.Handle("/", s.spaHandler())
	}

	return corsMiddleware(s.config.CORS.AllowedOrigins, mux)
}

func (s *Server) spaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		path = strings.TrimRight(path, "/")

		// Try exact file match first (JS, CSS, images, etc.)
		if path != "" {
			if info, err := fs.Stat(s.frontendFS, path); err == nil && !info.IsDir() {
				http.FileServerFS(s.frontendFS).ServeHTTP(w, r)
				return
			}
		}

		// Next.js static export naming: /sessions -> sessions.html
		// Dynamic routes: /instances/abc -> instances/_.html, /sessions/abc -> sessions/_.html
		candidates := []string{"index.html"}
		if path != "" {
			// Try path.html (e.g., sessions -> sessions.html)
			candidates = []string{path + ".html"}
			// Try path/index.html
			candidates = append(candidates, path+"/index.html")
			// For dynamic routes like instances/abc, try instances/_.html
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				candidates = append(candidates, path[:idx]+"/_.html")
			}
			// Fallback to index.html for SPA routing
			candidates = append(candidates, "index.html")
		}

		for _, candidate := range candidates {
			if f, err := s.frontendFS.Open(candidate); err == nil {
				f.Close()
				data, err := fs.ReadFile(s.frontendFS, candidate)
				if err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
		}

		// Final fallback
		data, err := fs.ReadFile(s.frontendFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
}

// handleWS upgrades an HTTP connection to a WebSocket, registers the client
// with the hub, and manages the read/write loops.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	ch := make(chan []byte, 256)
	s.hub.Register(ch)

	// Write loop: send hub broadcasts to the WebSocket client.
	go func() {
		defer conn.Close()
		for msg := range ch {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	// Read loop: discard incoming messages, detect close.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.hub.Unregister(ch)
}

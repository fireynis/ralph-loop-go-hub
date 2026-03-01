package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/fireynis/ralph-hub/internal/events"
	"github.com/fireynis/ralph-hub/internal/store"
)

// writeJSON writes data as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handlePostEvent receives a new event, validates it, persists it,
// broadcasts it via WebSocket, and dispatches webhooks.
func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	var evt events.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := evt.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.SaveEvent(r.Context(), evt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save event")
		return
	}

	// Broadcast to WebSocket clients.
	data, err := json.Marshal(evt)
	if err == nil {
		s.hub.Broadcast(data)
	}

	// Dispatch webhooks asynchronously.
	s.dispatcher.Dispatch(evt)

	writeJSON(w, http.StatusCreated, evt)
}

// handleGetInstances returns all currently active instances.
func (s *Server) handleGetInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := s.store.GetActiveInstances(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get instances")
		return
	}

	// Return empty array instead of null.
	if instances == nil {
		instances = []store.InstanceState{}
	}

	writeJSON(w, http.StatusOK, instances)
}

// handleGetInstanceHistory returns the iteration history for a specific instance.
func (s *Server) handleGetInstanceHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "instance id is required")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	records, err := s.store.GetInstanceHistory(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get instance history")
		return
	}

	if records == nil {
		records = []store.IterationRecord{}
	}

	writeJSON(w, http.StatusOK, records)
}

// handleGetSessions returns a paginated list of sessions.
func (s *Server) handleGetSessions(w http.ResponseWriter, r *http.Request) {
	filter := store.SessionFilter{}

	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err == nil && parsed > 0 {
			filter.Limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		parsed, err := strconv.Atoi(o)
		if err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	sessions, err := s.store.GetSessions(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get sessions")
		return
	}

	if sessions == nil {
		sessions = []store.Session{}
	}

	writeJSON(w, http.StatusOK, sessions)
}

// handleGetSessionDetail returns the full detail for a single session.
func (s *Server) handleGetSessionDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	detail, err := s.store.GetSessionDetail(r.Context(), id)
	if err != nil {
		// The store returns an error containing "not found" when the session doesn't exist.
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// handleGetStats returns aggregate statistics across all sessions.
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetAggregateStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

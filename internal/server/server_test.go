package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
)

// testServer creates a test HTTP server backed by an in-memory SQLite store.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	return testServerWithConfig(t, config.Config{
		Auth: config.AuthConfig{
			APIKeys: []config.APIKey{{Name: "test", Key: "test-key"}},
		},
		CORS: config.CORSConfig{
			AllowedOrigins: []string{"http://localhost:3000"},
		},
	})
}

// testServerWithConfig creates a test HTTP server with a custom config.
func testServerWithConfig(t *testing.T, cfg config.Config) *httptest.Server {
	t.Helper()
	st, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	h := ws.NewHub()
	d := webhook.New(nil)
	srv := New(cfg, st, h, d)
	return httptest.NewServer(srv.Handler())
}

// validEventJSON returns a JSON-encoded event with all required fields.
func validEventJSON() []byte {
	return []byte(`{
		"event_id": "evt-001",
		"type": "session.started",
		"timestamp": "` + time.Now().UTC().Format(time.RFC3339Nano) + `",
		"instance_id": "inst-1",
		"repo": "my-repo",
		"epic": "epic-1",
		"context": {
			"session_id": "sess-001",
			"session_start": "` + time.Now().UTC().Format(time.RFC3339Nano) + `",
			"max_iterations": 10,
			"current_iteration": 0,
			"status": "running",
			"current_phase": "planning"
		}
	}`)
}

func TestPostEvent_Unauthorized(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/events", "application/json", bytes.NewReader(validEventJSON()))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %q", body["error"])
	}
}

func TestPostEvent_Success(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader(validEventJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestPostEvent_InvalidJSON(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader([]byte(`{not valid`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPostEvent_ValidationError(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Missing required fields (no event_id, no type, etc.)
	body := []byte(`{"repo": "test"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetInstances_Empty(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/instances")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var instances []store.InstanceState
	json.NewDecoder(resp.Body).Decode(&instances)
	if instances == nil {
		t.Error("expected empty array, got null")
	}
	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}
}

func TestGetInstances_WithData(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Post an event first.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader(validEventJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on post, got %d", resp.StatusCode)
	}

	// Now get instances.
	resp, err = http.Get(ts.URL + "/api/v1/instances")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var instances []store.InstanceState
	json.NewDecoder(resp.Body).Decode(&instances)
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].InstanceID != "inst-1" {
		t.Errorf("expected instance_id 'inst-1', got %q", instances[0].InstanceID)
	}
}

func TestGetStats(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Post a session.started event.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader(validEventJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	resp.Body.Close()

	// Get stats.
	resp, err = http.Get(ts.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var stats store.AggregateStats
	json.NewDecoder(resp.Body).Decode(&stats)
	if stats.TotalSessions != 1 {
		t.Errorf("expected 1 total session, got %d", stats.TotalSessions)
	}
	if stats.ActiveInstances != 1 {
		t.Errorf("expected 1 active instance, got %d", stats.ActiveInstances)
	}
}

func TestHealthz(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCORS_PreflightRequest(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("OPTIONS", ts.URL+"/api/v1/instances", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected Allow-Origin 'http://localhost:3000', got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got != "GET, POST, OPTIONS" {
		t.Errorf("expected Allow-Methods 'GET, POST, OPTIONS', got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("expected Allow-Headers 'Content-Type, Authorization', got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("expected Max-Age '86400', got %q", got)
	}
}

func TestCORS_AllowedOrigin(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/instances", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected Allow-Origin 'http://localhost:3000', got %q", got)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/instances", nil)
	req.Header.Set("Origin", "http://evil.example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin header, got %q", got)
	}
}

func TestEndSession_Success(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a running session via event ingestion.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader(validEventJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	resp.Body.Close()

	// End the session.
	body := []byte(`{"reason": "stale"}`)
	resp, err = http.Post(ts.URL+"/api/v1/sessions/sess-001/end", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("end session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var detail store.SessionDetail
	json.NewDecoder(resp.Body).Decode(&detail)
	if detail.Session.EndedAt == nil {
		t.Error("expected session to have ended_at set")
	}
}

func TestEndSession_NotFound(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/sessions/nonexistent/end", "application/json", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestEndSession_AlreadyEnded(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a session.
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/events", bytes.NewReader(validEventJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	resp.Body.Close()

	// End it once.
	resp, err = http.Post(ts.URL+"/api/v1/sessions/sess-001/end", "application/json", nil)
	if err != nil {
		t.Fatalf("first end: %v", err)
	}
	resp.Body.Close()

	// End it again — should be 409.
	resp, err = http.Post(ts.URL+"/api/v1/sessions/sess-001/end", "application/json", nil)
	if err != nil {
		t.Fatalf("second end: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

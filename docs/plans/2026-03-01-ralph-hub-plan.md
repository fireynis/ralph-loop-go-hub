# Ralph Hub Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a centralized event hub and dashboard for monitoring multiple Ralph Loop instances across repositories — real-time WebSocket updates, historical reporting, and configurable webhook relay.

**Architecture:** Go HTTP server receives events from Ralph instances via POST, stores in SQLite or PostgreSQL, pushes to browser dashboards via WebSocket, and optionally relays to outbound webhooks (Slack, Discord, custom). Next.js frontend with Zustand for state management.

**Tech Stack:** Go 1.25+, gorilla/websocket, database/sql + modernc.org/sqlite + lib/pq, gopkg.in/yaml.v3, Next.js 15, React, Tailwind CSS, Zustand, Recharts

**Design doc:** See `ralph-loop-tui/docs/plans/2026-03-01-ralph-hub-design.md`

---

## Phase 1: Go Server Core

### Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `cmd/hub/main.go`
- Create: `Makefile`

**Step 1: Initialize the Go module**

```bash
cd ralph-hub
go mod init github.com/fireynis/ralph-hub
```

**Step 2: Create entry point**

Create `cmd/hub/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("ralph-hub starting...")
	os.Exit(0)
}
```

**Step 3: Create Makefile**

```makefile
.PHONY: build lint clean run

build:
	go build -o ralph-hub ./cmd/hub

lint:
	golangci-lint run

clean:
	rm -f ralph-hub

run: build
	./ralph-hub
```

**Step 4: Build and verify**

Run: `make build && ./ralph-hub`
Expected: Prints "ralph-hub starting..." and exits

**Step 5: Commit**

```bash
git init
git add .
git commit -m "feat: project scaffold with Go module and Makefile"
```

---

### Task 2: Configuration Loading

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config.example.yaml`

**Step 1: Write the failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("driver = %s, want sqlite", cfg.Storage.Driver)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  port: 9090
storage:
  driver: postgres
  postgres:
    dsn: postgres://localhost/test
auth:
  api_keys:
    - name: test
      key: rhk_test123
webhooks:
  - url: https://hooks.slack.com/xxx
    events: ["session.ended"]
`
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Storage.Driver != "postgres" {
		t.Errorf("driver = %s, want postgres", cfg.Storage.Driver)
	}
	if len(cfg.Auth.APIKeys) != 1 || cfg.Auth.APIKeys[0].Key != "rhk_test123" {
		t.Error("api keys not loaded correctly")
	}
	if len(cfg.Webhooks) != 1 {
		t.Errorf("webhooks count = %d, want 1", len(cfg.Webhooks))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL

**Step 3: Write implementation**

Create `internal/config/config.go`:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	Auth     AuthConfig     `yaml:"auth"`
	Webhooks []WebhookConfig `yaml:"webhooks"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type StorageConfig struct {
	Driver   string         `yaml:"driver"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Postgres PostgresConfig `yaml:"postgres"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type AuthConfig struct {
	APIKeys []APIKey `yaml:"api_keys"`
}

type APIKey struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type WebhookConfig struct {
	URL    string   `yaml:"url"`
	Events []string `yaml:"events"`
	Filter struct {
		PassedOnly bool `yaml:"passed_only"`
	} `yaml:"filter"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{Port: 8080},
		Storage: StorageConfig{
			Driver: "sqlite",
			SQLite: SQLiteConfig{Path: "./ralph-hub.db"},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	if path == "" {
		return &cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

Add dependency: `go get gopkg.in/yaml.v3`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 5: Create example config**

Create `config.example.yaml`:

```yaml
server:
  port: 8080

storage:
  driver: sqlite  # or postgres
  sqlite:
    path: ./ralph-hub.db
  postgres:
    dsn: postgres://user:pass@localhost/ralph_hub

auth:
  api_keys:
    - name: "my-laptop"
      key: "rhk_change-me"

webhooks: []
  # - url: https://hooks.slack.com/services/xxx
  #   events: ["session.ended", "iteration.completed"]
  #   filter:
  #     passed_only: false
```

**Step 6: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add YAML configuration loading with defaults"
```

---

### Task 3: Event Types (Shared)

**Files:**
- Create: `internal/events/events.go`
- Create: `internal/events/events_test.go`

**Step 1: Write the failing test**

```go
package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventRoundTrip(t *testing.T) {
	evt := Event{
		ID:         "evt_abc123",
		Type:       IterationCompleted,
		Timestamp:  time.Date(2026, 3, 1, 14, 0, 0, 0, time.UTC),
		InstanceID: "my-app/BD-42",
		Repo:       "my-app",
		Epic:       "BD-42",
		Data: map[string]any{
			"iteration":   float64(7),
			"duration_ms": float64(45000),
		},
		Context: Context{
			SessionID:        "sess_xyz",
			MaxIterations:    50,
			CurrentIteration: 7,
			Status:           "running",
			CurrentPhase:     "dev",
		},
	}

	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != IterationCompleted {
		t.Errorf("type = %s, want %s", decoded.Type, IterationCompleted)
	}
	if decoded.InstanceID != "my-app/BD-42" {
		t.Errorf("instance_id = %s, want my-app/BD-42", decoded.InstanceID)
	}
}

func TestEventValidation(t *testing.T) {
	tests := []struct {
		name    string
		evt     Event
		wantErr bool
	}{
		{"valid", Event{ID: "e1", Type: SessionStarted, InstanceID: "r", Repo: "r"}, false},
		{"missing type", Event{ID: "e1", InstanceID: "r", Repo: "r"}, true},
		{"missing instance_id", Event{ID: "e1", Type: SessionStarted, Repo: "r"}, true},
		{"missing repo", Event{ID: "e1", Type: SessionStarted, InstanceID: "r"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.evt.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/events/ -v`
Expected: FAIL

**Step 3: Write implementation**

Create `internal/events/events.go`:

```go
package events

import (
	"errors"
	"time"
)

type Type string

const (
	SessionStarted     Type = "session.started"
	SessionEnded       Type = "session.ended"
	IterationStarted   Type = "iteration.started"
	IterationCompleted Type = "iteration.completed"
	PhaseChanged       Type = "phase.changed"
	TaskClaimed        Type = "task.claimed"
	TaskClosed         Type = "task.closed"
)

type Event struct {
	ID         string         `json:"event_id"`
	Type       Type           `json:"type"`
	Timestamp  time.Time      `json:"timestamp"`
	InstanceID string         `json:"instance_id"`
	Repo       string         `json:"repo"`
	Epic       string         `json:"epic,omitempty"`
	Data       map[string]any `json:"data"`
	Context    Context        `json:"context"`
}

type Context struct {
	SessionID        string    `json:"session_id"`
	SessionStart     time.Time `json:"session_start"`
	MaxIterations    int       `json:"max_iterations"`
	CurrentIteration int       `json:"current_iteration"`
	Status           string    `json:"status"`
	CurrentPhase     string    `json:"current_phase"`
	Analytics        Analytics `json:"analytics"`
}

type Analytics struct {
	PassedCount     int   `json:"passed_count"`
	FailedCount     int   `json:"failed_count"`
	TasksClosed     int   `json:"tasks_closed"`
	InitialReady    int   `json:"initial_ready"`
	CurrentReady    int   `json:"current_ready"`
	AvgDurationMs   int64 `json:"avg_duration_ms"`
	TotalDurationMs int64 `json:"total_duration_ms"`
}

func (e *Event) Validate() error {
	if e.Type == "" {
		return errors.New("event type is required")
	}
	if e.InstanceID == "" {
		return errors.New("instance_id is required")
	}
	if e.Repo == "" {
		return errors.New("repo is required")
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/events/ -v`
Expected: PASS

**Step 5: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add event types with validation"
```

---

### Task 4: Store Interface and SQLite Implementation

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/sqlite.go`
- Create: `internal/store/sqlite_test.go`

**Step 1: Write the store interface**

Create `internal/store/store.go`:

```go
package store

import (
	"context"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
)

type InstanceState struct {
	InstanceID   string         `json:"instance_id"`
	Repo         string         `json:"repo"`
	Epic         string         `json:"epic,omitempty"`
	Status       string         `json:"status"`
	LastEvent    time.Time      `json:"last_event"`
	Context      events.Context `json:"context"`
}

type IterationRecord struct {
	Iteration    int           `json:"iteration"`
	DurationMs   int64         `json:"duration_ms"`
	TaskID       string        `json:"task_id"`
	Passed       bool          `json:"passed"`
	Notes        string        `json:"notes"`
	ReviewCycles int           `json:"review_cycles"`
	FinalVerdict string        `json:"final_verdict"`
	Timestamp    time.Time     `json:"timestamp"`
}

type Session struct {
	SessionID      string    `json:"session_id"`
	InstanceID     string    `json:"instance_id"`
	Repo           string    `json:"repo"`
	Epic           string    `json:"epic,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	Iterations     int       `json:"iterations"`
	TasksClosed    int       `json:"tasks_closed"`
	PassRate       float64   `json:"pass_rate"`
	EndReason      string    `json:"end_reason,omitempty"`
}

type SessionFilter struct {
	Repo      string
	DateFrom  *time.Time
	DateTo    *time.Time
	Limit     int
	Offset    int
}

type SessionDetail struct {
	Session Session        `json:"session"`
	Events  []events.Event `json:"events"`
}

type AggregateStats struct {
	TotalSessions    int     `json:"total_sessions"`
	ActiveInstances  int     `json:"active_instances"`
	TotalTasksClosed int     `json:"total_tasks_closed"`
	OverallPassRate  float64 `json:"overall_pass_rate"`
	TotalIterations  int     `json:"total_iterations"`
}

type Store interface {
	SaveEvent(ctx context.Context, event events.Event) error
	GetActiveInstances(ctx context.Context) ([]InstanceState, error)
	GetInstanceHistory(ctx context.Context, instanceID string, limit int) ([]IterationRecord, error)
	GetSessions(ctx context.Context, filter SessionFilter) ([]Session, error)
	GetSessionDetail(ctx context.Context, sessionID string) (*SessionDetail, error)
	GetAggregateStats(ctx context.Context) (*AggregateStats, error)
	Close() error
}
```

**Step 2: Write the failing test for SQLite**

Create `internal/store/sqlite_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
)

func newTestSQLite(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("new sqlite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLite_SaveAndGetActiveInstances(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	evt := events.Event{
		ID:         "evt_1",
		Type:       events.SessionStarted,
		Timestamp:  time.Now().UTC(),
		InstanceID: "my-app/BD-42",
		Repo:       "my-app",
		Epic:       "BD-42",
		Data:       map[string]any{"max_iterations": float64(50)},
		Context: events.Context{
			SessionID:     "sess_1",
			MaxIterations: 50,
			Status:        "running",
		},
	}

	if err := s.SaveEvent(ctx, evt); err != nil {
		t.Fatalf("save: %v", err)
	}

	instances, err := s.GetActiveInstances(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("count = %d, want 1", len(instances))
	}
	if instances[0].InstanceID != "my-app/BD-42" {
		t.Errorf("instance_id = %s, want my-app/BD-42", instances[0].InstanceID)
	}
}

func TestSQLite_IterationHistory(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		evt := events.Event{
			ID:         "evt_" + string(rune('0'+i)),
			Type:       events.IterationCompleted,
			Timestamp:  time.Now().UTC(),
			InstanceID: "my-app",
			Repo:       "my-app",
			Data: map[string]any{
				"iteration":     float64(i),
				"duration_ms":   float64(30000),
				"task_id":       "BD-" + string(rune('0'+i)),
				"passed":        true,
				"notes":         "done",
				"review_cycles": float64(1),
				"final_verdict": "APPROVED",
			},
			Context: events.Context{SessionID: "sess_1", Status: "running"},
		}
		s.SaveEvent(ctx, evt)
	}

	history, err := s.GetInstanceHistory(ctx, "my-app", 10)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(history) != 3 {
		t.Errorf("count = %d, want 3", len(history))
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL

**Step 4: Write SQLite implementation**

Create `internal/store/sqlite.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			instance_id TEXT NOT NULL,
			repo TEXT NOT NULL,
			epic TEXT DEFAULT '',
			data_json TEXT NOT NULL,
			context_json TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_events_instance ON events(instance_id);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

		CREATE TABLE IF NOT EXISTS instance_state (
			instance_id TEXT PRIMARY KEY,
			repo TEXT NOT NULL,
			epic TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'running',
			last_event DATETIME NOT NULL,
			context_json TEXT NOT NULL
		);
	`)
	return err
}

func (s *SQLiteStore) SaveEvent(ctx context.Context, evt events.Event) error {
	dataJSON, err := json.Marshal(evt.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	ctxJSON, err := json.Marshal(evt.Context)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO events (id, type, timestamp, instance_id, repo, epic, data_json, context_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		evt.ID, string(evt.Type), evt.Timestamp, evt.InstanceID, evt.Repo, evt.Epic, string(dataJSON), string(ctxJSON))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	status := evt.Context.Status
	if evt.Type == events.SessionEnded {
		status = "ended"
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO instance_state (instance_id, repo, epic, status, last_event, context_json)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(instance_id) DO UPDATE SET
		   status = excluded.status,
		   last_event = excluded.last_event,
		   context_json = excluded.context_json`,
		evt.InstanceID, evt.Repo, evt.Epic, status, evt.Timestamp, string(ctxJSON))
	if err != nil {
		return fmt.Errorf("upsert instance: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetActiveInstances(ctx context.Context) ([]InstanceState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT instance_id, repo, epic, status, last_event, context_json
		 FROM instance_state ORDER BY last_event DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []InstanceState
	for rows.Next() {
		var inst InstanceState
		var ctxJSON string
		if err := rows.Scan(&inst.InstanceID, &inst.Repo, &inst.Epic, &inst.Status, &inst.LastEvent, &ctxJSON); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(ctxJSON), &inst.Context)
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

func (s *SQLiteStore) GetInstanceHistory(ctx context.Context, instanceID string, limit int) ([]IterationRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data_json, timestamp FROM events
		 WHERE instance_id = ? AND type = ?
		 ORDER BY timestamp DESC LIMIT ?`,
		instanceID, string(events.IterationCompleted), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []IterationRecord
	for rows.Next() {
		var dataJSON string
		var ts time.Time
		if err := rows.Scan(&dataJSON, &ts); err != nil {
			return nil, err
		}
		var data map[string]any
		json.Unmarshal([]byte(dataJSON), &data)

		rec := IterationRecord{Timestamp: ts}
		if v, ok := data["iteration"].(float64); ok {
			rec.Iteration = int(v)
		}
		if v, ok := data["duration_ms"].(float64); ok {
			rec.DurationMs = int64(v)
		}
		if v, ok := data["task_id"].(string); ok {
			rec.TaskID = v
		}
		if v, ok := data["passed"].(bool); ok {
			rec.Passed = v
		}
		if v, ok := data["notes"].(string); ok {
			rec.Notes = v
		}
		if v, ok := data["review_cycles"].(float64); ok {
			rec.ReviewCycles = int(v)
		}
		if v, ok := data["final_verdict"].(string); ok {
			rec.FinalVerdict = v
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) GetSessions(ctx context.Context, filter SessionFilter) ([]Session, error) {
	query := `
		SELECT
			e_start.context_json,
			e_start.instance_id,
			e_start.repo,
			e_start.epic,
			e_start.timestamp as started_at,
			e_end.timestamp as ended_at,
			e_end.data_json as end_data
		FROM events e_start
		LEFT JOIN events e_end ON
			json_extract(e_start.context_json, '$.session_id') = json_extract(e_end.context_json, '$.session_id')
			AND e_end.type = 'session.ended'
		WHERE e_start.type = 'session.started'
		ORDER BY e_start.timestamp DESC
		LIMIT ? OFFSET ?`

	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, query, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		var ctxJSON string
		var endedAt sql.NullTime
		var endDataJSON sql.NullString

		if err := rows.Scan(&ctxJSON, &s.InstanceID, &s.Repo, &s.Epic, &s.StartedAt, &endedAt, &endDataJSON); err != nil {
			return nil, err
		}

		var evtCtx events.Context
		json.Unmarshal([]byte(ctxJSON), &evtCtx)
		s.SessionID = evtCtx.SessionID
		s.Iterations = evtCtx.CurrentIteration
		s.TasksClosed = evtCtx.Analytics.TasksClosed

		total := evtCtx.Analytics.PassedCount + evtCtx.Analytics.FailedCount
		if total > 0 {
			s.PassRate = float64(evtCtx.Analytics.PassedCount) / float64(total)
		}

		if endedAt.Valid {
			s.EndedAt = &endedAt.Time
		}
		if endDataJSON.Valid {
			var endData map[string]any
			json.Unmarshal([]byte(endDataJSON.String), &endData)
			if reason, ok := endData["reason"].(string); ok {
				s.EndReason = reason
			}
		}

		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (s *SQLiteStore) GetSessionDetail(ctx context.Context, sessionID string) (*SessionDetail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, timestamp, instance_id, repo, epic, data_json, context_json
		 FROM events
		 WHERE json_extract(context_json, '$.session_id') = ?
		 ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	detail := &SessionDetail{}
	for rows.Next() {
		var evt events.Event
		var dataJSON, ctxJSON string
		if err := rows.Scan(&evt.ID, &evt.Type, &evt.Timestamp, &evt.InstanceID, &evt.Repo, &evt.Epic, &dataJSON, &ctxJSON); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(dataJSON), &evt.Data)
		json.Unmarshal([]byte(ctxJSON), &evt.Context)
		detail.Events = append(detail.Events, evt)
	}

	if len(detail.Events) == 0 {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Build session summary from first and last events
	first := detail.Events[0]
	last := detail.Events[len(detail.Events)-1]
	detail.Session = Session{
		SessionID:  sessionID,
		InstanceID: first.InstanceID,
		Repo:       first.Repo,
		Epic:       first.Epic,
		StartedAt:  first.Timestamp,
		Iterations: last.Context.CurrentIteration,
		TasksClosed: last.Context.Analytics.TasksClosed,
	}
	if last.Type == events.SessionEnded {
		detail.Session.EndedAt = &last.Timestamp
	}
	total := last.Context.Analytics.PassedCount + last.Context.Analytics.FailedCount
	if total > 0 {
		detail.Session.PassRate = float64(last.Context.Analytics.PassedCount) / float64(total)
	}

	return detail, rows.Err()
}

func (s *SQLiteStore) GetAggregateStats(ctx context.Context) (*AggregateStats, error) {
	stats := &AggregateStats{}

	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'session.started'`).Scan(&stats.TotalSessions)

	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instance_state WHERE status = 'running'`).Scan(&stats.ActiveInstances)

	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'`).Scan(&stats.TotalIterations)

	var passed, failed int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'
		 AND json_extract(data_json, '$.passed') = true`).Scan(&passed)
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'
		 AND json_extract(data_json, '$.passed') = false`).Scan(&failed)

	stats.TotalTasksClosed = passed
	if passed+failed > 0 {
		stats.OverallPassRate = float64(passed) / float64(passed+failed)
	}

	return stats, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
```

Add dependency: `go get modernc.org/sqlite`

**Step 5: Run test to verify it passes**

Run: `go test ./internal/store/ -v`
Expected: PASS

**Step 6: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add Store interface and SQLite implementation"
```

---

### Task 5: PostgreSQL Store Implementation

**Files:**
- Create: `internal/store/postgres.go`
- Create: `internal/store/postgres_test.go`

**Step 1: Write the implementation**

Create `internal/store/postgres.go` following the same interface as SQLite but using `lib/pq` and PostgreSQL-specific SQL (e.g., `jsonb` columns, `ON CONFLICT DO UPDATE`, `$1` parameter placeholders).

Key differences from SQLite:
- Use `$1, $2, ...` placeholders instead of `?`
- Use `jsonb` type for data/context columns
- Use `->>'key'` for JSON extraction instead of `json_extract()`
- Migrations use `CREATE TABLE IF NOT EXISTS` with the same schema

Add dependency: `go get github.com/lib/pq`

**Step 2: Write integration test**

The test should be skipped unless `RALPH_TEST_POSTGRES_DSN` is set:

```go
func TestPostgres_SaveAndGet(t *testing.T) {
	dsn := os.Getenv("RALPH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RALPH_TEST_POSTGRES_DSN not set")
	}
	// Same test logic as SQLite
}
```

**Step 3: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add PostgreSQL store implementation"
```

---

### Task 6: WebSocket Hub

**Files:**
- Create: `internal/ws/hub.go`
- Create: `internal/ws/hub_test.go`

**Step 1: Write the failing test**

```go
package ws

import (
	"testing"

	"github.com/fireynis/ralph-hub/internal/events"
)

func TestHub_BroadcastToClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Create a mock client channel
	ch := make(chan []byte, 10)
	hub.Register(ch)
	defer hub.Unregister(ch)

	evt := events.Event{
		ID:         "evt_1",
		Type:       events.IterationCompleted,
		InstanceID: "test",
		Repo:       "test",
	}

	hub.Broadcast(evt)

	msg := <-ch
	if len(msg) == 0 {
		t.Error("received empty message")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ws/ -v`
Expected: FAIL

**Step 3: Write implementation**

Create `internal/ws/hub.go`:

```go
package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/fireynis/ralph-hub/internal/events"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan []byte]struct{}),
	}
}

func (h *Hub) Register(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
}

func (h *Hub) Unregister(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
	close(ch)
}

func (h *Hub) Broadcast(evt events.Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[ws] marshal error: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- data:
		default:
			// Client buffer full, skip
			log.Printf("[ws] client buffer full, dropping message")
		}
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Run is a no-op kept for API compatibility; Hub uses mutex-based synchronization.
func (h *Hub) Run() {}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/ws/ -v`
Expected: PASS

**Step 5: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add WebSocket hub for real-time broadcast"
```

---

### Task 7: Webhook Dispatcher

**Files:**
- Create: `internal/webhook/dispatcher.go`
- Create: `internal/webhook/dispatcher_test.go`

**Step 1: Write the failing test**

```go
package webhook

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/events"
)

func TestDispatcher_SendsToMatchingWebhooks(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hooks := []config.WebhookConfig{
		{URL: srv.URL, Events: []string{"session.ended"}},
	}

	d := New(hooks)
	d.Dispatch(events.Event{Type: events.SessionEnded, InstanceID: "test", Repo: "test"})
	time.Sleep(100 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("received = %d, want 1", received.Load())
	}
}

func TestDispatcher_FiltersNonMatchingEvents(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hooks := []config.WebhookConfig{
		{URL: srv.URL, Events: []string{"session.ended"}},
	}

	d := New(hooks)
	// Send a non-matching event
	d.Dispatch(events.Event{Type: events.IterationStarted, InstanceID: "test", Repo: "test"})
	time.Sleep(100 * time.Millisecond)

	if received.Load() != 0 {
		t.Errorf("received = %d, want 0", received.Load())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webhook/ -v`
Expected: FAIL

**Step 3: Write implementation**

Create `internal/webhook/dispatcher.go`:

```go
package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/events"
)

type Dispatcher struct {
	hooks  []config.WebhookConfig
	client *http.Client
}

func New(hooks []config.WebhookConfig) *Dispatcher {
	return &Dispatcher{
		hooks:  hooks,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Dispatcher) Dispatch(evt events.Event) {
	for _, hook := range d.hooks {
		if !d.matches(hook, evt) {
			continue
		}
		go d.deliver(hook, evt)
	}
}

func (d *Dispatcher) matches(hook config.WebhookConfig, evt events.Event) bool {
	for _, allowed := range hook.Events {
		if allowed == string(evt.Type) {
			return true
		}
	}
	return false
}

func (d *Dispatcher) deliver(hook config.WebhookConfig, evt events.Event) {
	body, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[webhook] marshal error: %v", err)
		return
	}

	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := d.client.Post(hook.URL, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[webhook] attempt %d/%d to %s failed: %v", attempt, maxRetries, hook.URL, err)
		} else {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return // Success
			}
			log.Printf("[webhook] attempt %d/%d to %s returned %d", attempt, maxRetries, hook.URL, resp.StatusCode)
		}

		if attempt < maxRetries {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}
	}

	log.Printf("[webhook] gave up delivering to %s after %d attempts", hook.URL, maxRetries)
	fmt.Printf("[webhook] FAILED: delivery to %s for event %s\n", hook.URL, evt.Type)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/webhook/ -v`
Expected: PASS

**Step 5: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add webhook dispatcher with retry logic"
```

---

### Task 8: HTTP Server and Routes

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/handlers.go`
- Create: `internal/server/middleware.go`
- Create: `internal/server/server_test.go`

**Step 1: Write the failing test**

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/events"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	st, _ := store.NewSQLiteStore(":memory:")
	t.Cleanup(func() { st.Close() })

	cfg := &config.Config{
		Auth: config.AuthConfig{
			APIKeys: []config.APIKey{{Name: "test", Key: "test-key"}},
		},
	}

	return New(cfg, st, ws.NewHub(), webhook.New(nil))
}

func TestPostEvent_Success(t *testing.T) {
	srv := newTestServer(t)

	evt := events.Event{
		ID:         "evt_1",
		Type:       events.SessionStarted,
		Timestamp:  time.Now().UTC(),
		InstanceID: "test-app",
		Repo:       "test-app",
		Data:       map[string]any{},
		Context:    events.Context{SessionID: "s1"},
	}
	body, _ := json.Marshal(evt)

	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != 202 {
		t.Errorf("status = %d, want 202", rec.Code)
	}
}

func TestPostEvent_Unauthorized(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestGetInstances(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/instances", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -v`
Expected: FAIL

**Step 3: Write implementation**

Create `internal/server/middleware.go`:

```go
package server

import (
	"net/http"
	"strings"

	"github.com/fireynis/ralph-hub/internal/config"
)

func authMiddleware(keys []config.APIKey) func(http.Handler) http.Handler {
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k.Key] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if !keySet[token] {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

Create `internal/server/handlers.go`:

```go
package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/fireynis/ralph-hub/internal/events"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
)

type handlers struct {
	store      store.Store
	hub        *ws.Hub
	dispatcher *webhook.Dispatcher
}

func (h *handlers) postEvent(w http.ResponseWriter, r *http.Request) {
	var evt events.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := evt.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.SaveEvent(r.Context(), evt); err != nil {
		log.Printf("[server] save error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.hub.Broadcast(evt)
	h.dispatcher.Dispatch(evt)

	w.WriteHeader(http.StatusAccepted)
}

func (h *handlers) getInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := h.store.GetActiveInstances(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if instances == nil {
		instances = []store.InstanceState{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(instances)
}

func (h *handlers) getInstanceHistory(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	records, err := h.store.GetInstanceHistory(r.Context(), instanceID, 100)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []store.IterationRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

func (h *handlers) getSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.store.GetSessions(r.Context(), store.SessionFilter{Limit: 50})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []store.Session{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func (h *handlers) getSessionDetail(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	detail, err := h.store.GetSessionDetail(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func (h *handlers) getStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetAggregateStats(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
```

Create `internal/server/server.go`:

```go
package server

import (
	"log"
	"net/http"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
	"github.com/gorilla/websocket"
)

type Server struct {
	cfg        *config.Config
	store      store.Store
	hub        *ws.Hub
	dispatcher *webhook.Dispatcher
}

func New(cfg *config.Config, st store.Store, hub *ws.Hub, disp *webhook.Dispatcher) *Server {
	return &Server{cfg: cfg, store: st, hub: hub, dispatcher: disp}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] upgrade error: %v", err)
		return
	}

	ch := make(chan []byte, 256)
	s.hub.Register(ch)

	// Writer goroutine
	go func() {
		defer conn.Close()
		for msg := range ch {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				break
			}
		}
	}()

	// Reader goroutine (just for detecting disconnect)
	go func() {
		defer s.hub.Unregister(ch)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	h := &handlers{store: s.store, hub: s.hub, dispatcher: s.dispatcher}
	auth := authMiddleware(s.cfg.Auth.APIKeys)

	// Ingestion (requires auth)
	mux.Handle("POST /api/v1/events", auth(http.HandlerFunc(h.postEvent)))

	// Query endpoints (no auth for dashboard)
	mux.HandleFunc("GET /api/v1/instances", h.getInstances)
	mux.HandleFunc("GET /api/v1/instances/{id}/history", h.getInstanceHistory)
	mux.HandleFunc("GET /api/v1/sessions", h.getSessions)
	mux.HandleFunc("GET /api/v1/sessions/{id}", h.getSessionDetail)
	mux.HandleFunc("GET /api/v1/stats", h.getStats)

	// WebSocket
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)

	return mux
}

func (s *Server) ListenAndServe(addr string) error {
	log.Printf("[server] listening on %s", addr)
	return http.ListenAndServe(addr, s.Handler())
}
```

Add dependency: `go get github.com/gorilla/websocket`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -v`
Expected: PASS

**Step 5: Lint and commit**

```bash
golangci-lint run
git add .
git commit -m "feat: add HTTP server with API routes, auth, and WebSocket"
```

---

### Task 9: Wire Up cmd/hub/main.go

**Files:**
- Modify: `cmd/hub/main.go`

**Step 1: Wire everything together**

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/server"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// If default config doesn't exist, use defaults
		if *configPath == "config.yaml" {
			cfg, _ = config.Load("")
		} else {
			log.Fatalf("load config: %v", err)
		}
	}

	var st store.Store
	switch cfg.Storage.Driver {
	case "sqlite":
		st, err = store.NewSQLiteStore(cfg.Storage.SQLite.Path)
	case "postgres":
		st, err = store.NewPostgresStore(cfg.Storage.Postgres.DSN)
	default:
		log.Fatalf("unknown storage driver: %s", cfg.Storage.Driver)
	}
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer st.Close()

	hub := ws.NewHub()
	disp := webhook.New(cfg.Webhooks)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := server.New(cfg, st, hub, disp)

	fmt.Fprintf(os.Stderr, "ralph-hub listening on %s (storage: %s)\n", addr, cfg.Storage.Driver)
	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

**Step 2: Build and verify**

Run: `make build && ./ralph-hub --help`
Expected: Shows flags, starts with defaults

**Step 3: Smoke test**

```bash
# Terminal 1: start the server
./ralph-hub

# Terminal 2: post an event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Authorization: Bearer rhk_change-me" \
  -H "Content-Type: application/json" \
  -d '{"event_id":"test","type":"session.started","instance_id":"test","repo":"test","data":{},"context":{"session_id":"s1"}}'

# Should get 202
# Then:
curl http://localhost:8080/api/v1/instances
# Should return array with "test" instance
```

**Step 4: Commit**

```bash
git add .
git commit -m "feat: wire up main entry point with config, store, hub, and server"
```

---

## Phase 2: Next.js Dashboard

### Task 10: Next.js Project Scaffold

**Step 1: Create Next.js app**

```bash
cd ralph-hub
npx create-next-app@latest web --typescript --tailwind --app --src-dir --no-eslint
cd web
npm install zustand recharts
```

**Step 2: Commit**

```bash
git add web/
git commit -m "feat: scaffold Next.js dashboard app"
```

---

### Task 11: WebSocket Hook and State Store

**Files:**
- Create: `web/src/hooks/useWebSocket.ts`
- Create: `web/src/store/instances.ts`
- Create: `web/src/lib/types.ts`

**Step 1: Create shared types**

Create `web/src/lib/types.ts`:

```typescript
export interface EventAnalytics {
  passed_count: number;
  failed_count: number;
  tasks_closed: number;
  initial_ready: number;
  current_ready: number;
  avg_duration_ms: number;
  total_duration_ms: number;
}

export interface EventContext {
  session_id: string;
  session_start: string;
  max_iterations: number;
  current_iteration: number;
  status: string;
  current_phase: string;
  analytics: EventAnalytics;
}

export interface RalphEvent {
  event_id: string;
  type: string;
  timestamp: string;
  instance_id: string;
  repo: string;
  epic?: string;
  data: Record<string, unknown>;
  context: EventContext;
}

export interface InstanceState {
  instance_id: string;
  repo: string;
  epic?: string;
  status: string;
  last_event: string;
  context: EventContext;
}

export interface Session {
  session_id: string;
  instance_id: string;
  repo: string;
  epic?: string;
  started_at: string;
  ended_at?: string;
  iterations: number;
  tasks_closed: number;
  pass_rate: number;
  end_reason?: string;
}

export interface AggregateStats {
  total_sessions: number;
  active_instances: number;
  total_tasks_closed: number;
  overall_pass_rate: number;
  total_iterations: number;
}
```

**Step 2: Create Zustand store**

Create `web/src/store/instances.ts`:

```typescript
import { create } from 'zustand';
import type { InstanceState, RalphEvent } from '@/lib/types';

interface InstanceStore {
  instances: Map<string, InstanceState>;
  recentEvents: RalphEvent[];
  setInstances: (instances: InstanceState[]) => void;
  handleEvent: (event: RalphEvent) => void;
}

export const useInstanceStore = create<InstanceStore>((set) => ({
  instances: new Map(),
  recentEvents: [],

  setInstances: (instances) =>
    set({
      instances: new Map(instances.map((i) => [i.instance_id, i])),
    }),

  handleEvent: (event) =>
    set((state) => {
      const instances = new Map(state.instances);
      instances.set(event.instance_id, {
        instance_id: event.instance_id,
        repo: event.repo,
        epic: event.epic,
        status: event.context.status,
        last_event: event.timestamp,
        context: event.context,
      });

      const recentEvents = [event, ...state.recentEvents].slice(0, 100);

      return { instances, recentEvents };
    }),
}));
```

**Step 3: Create WebSocket hook**

Create `web/src/hooks/useWebSocket.ts`:

```typescript
'use client';

import { useEffect, useRef, useCallback } from 'react';
import { useInstanceStore } from '@/store/instances';
import type { RalphEvent, InstanceState } from '@/lib/types';

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
const WS_URL = API_BASE.replace(/^http/, 'ws') + '/api/v1/ws';

export function useRalphWebSocket() {
  const wsRef = useRef<WebSocket | null>(null);
  const { setInstances, handleEvent } = useInstanceStore();

  const connect = useCallback(() => {
    // Fetch initial state
    fetch(`${API_BASE}/api/v1/instances`)
      .then((r) => r.json())
      .then((data: InstanceState[]) => setInstances(data))
      .catch((err) => console.error('Failed to fetch instances:', err));

    // Connect WebSocket
    const ws = new WebSocket(WS_URL);
    wsRef.current = ws;

    ws.onmessage = (msg) => {
      const event: RalphEvent = JSON.parse(msg.data);
      handleEvent(event);
    };

    ws.onclose = () => {
      // Reconnect after 3 seconds
      setTimeout(connect, 3000);
    };

    ws.onerror = (err) => {
      console.error('WebSocket error:', err);
      ws.close();
    };
  }, [setInstances, handleEvent]);

  useEffect(() => {
    connect();
    return () => wsRef.current?.close();
  }, [connect]);
}
```

**Step 4: Commit**

```bash
git add web/src/
git commit -m "feat: add WebSocket hook, Zustand store, and shared types"
```

---

### Task 12: Overview Dashboard Page

**Files:**
- Modify: `web/src/app/page.tsx`
- Create: `web/src/app/layout-client.tsx`
- Create: `web/src/components/instance-card.tsx`

**Step 1: Create the instance card component**

Create `web/src/components/instance-card.tsx` — a card showing repo name, epic, iteration progress, current phase, pass/fail rate, last task. Color-coded by status (green=running healthy, yellow=recent failures, gray=ended).

**Step 2: Create the layout client wrapper**

Create `web/src/app/layout-client.tsx` — wraps children and calls `useRalphWebSocket()` to establish the connection.

**Step 3: Update the overview page**

Modify `web/src/app/page.tsx` — grid of InstanceCard components, active instances on top, inactive collapsed below.

**Step 4: Test manually**

```bash
cd web && npm run dev
# Open http://localhost:3000
# Start ralph-hub in another terminal
# Verify cards appear when events are posted
```

**Step 5: Commit**

```bash
git add web/
git commit -m "feat: add overview dashboard page with instance cards"
```

---

### Task 13: Instance Detail Page

**Files:**
- Create: `web/src/app/instances/[id]/page.tsx`
- Create: `web/src/components/iteration-table.tsx`
- Create: `web/src/components/phase-indicator.tsx`

Build the instance detail page with:
- Live phase indicator (planner → dev → reviewer → fixer)
- Iteration history table
- Pass rate and duration trend charts using Recharts

**Commit:**
```bash
git add web/
git commit -m "feat: add instance detail page with history and charts"
```

---

### Task 14: Sessions Pages

**Files:**
- Create: `web/src/app/sessions/page.tsx`
- Create: `web/src/app/sessions/[id]/page.tsx`

Build:
- Sessions list page with table (repo, epic, started, ended, iterations, tasks closed, pass rate)
- Session detail page with full event timeline and summary stats

**Commit:**
```bash
git add web/
git commit -m "feat: add sessions list and detail pages"
```

---

### Task 15: Settings Page

**Files:**
- Create: `web/src/app/settings/page.tsx`

Build settings page for viewing webhook configuration. This is read-only initially — shows configured webhooks from the server.

**Commit:**
```bash
git add web/
git commit -m "feat: add settings page for webhook configuration"
```

---

## Phase 3: Final Integration

### Task 16: CORS Middleware

**Files:**
- Modify: `internal/server/middleware.go`

Add CORS middleware to allow the Next.js dev server (port 3000) to call the Go API (port 8080). In production, configure allowed origins via config.

**Commit:**
```bash
git add .
git commit -m "feat: add CORS middleware for dashboard cross-origin requests"
```

---

### Task 17: README and Documentation

**Files:**
- Create: `README.md`
- Create: `CLAUDE.md`

Write documentation covering:
- What ralph-hub is and how it works
- Quickstart (run locally)
- Configuration reference
- API reference
- How to connect Ralph Loop instances
- How to add webhooks
- Development setup (Go + Next.js)

**Commit:**
```bash
git add .
git commit -m "docs: add README and CLAUDE.md"
```

---

### Task 18: End-to-End Test

Manual integration test:

1. Start ralph-hub with SQLite: `./ralph-hub`
2. Start Next.js dashboard: `cd web && npm run dev`
3. Start ralph-loop-go with hub: `./ralph-loop-go -hub-url http://localhost:8080 -hub-api-key rhk_change-me -max-iterations 3`
4. Verify:
   - Dashboard shows the instance card appearing and updating in real-time
   - Iteration history populates as iterations complete
   - Phase transitions show live
   - Sessions page shows the session after it ends
   - Stats page shows aggregate numbers

```bash
git add .
git commit -m "docs: add end-to-end testing instructions"
```

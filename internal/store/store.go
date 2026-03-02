package store

import (
	"context"
	"errors"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
)

var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionAlreadyEnded = errors.New("session already ended")
)

// InstanceState represents the current state of a Ralph instance.
type InstanceState struct {
	InstanceID string          `json:"instance_id"`
	Repo       string          `json:"repo"`
	Epic       string          `json:"epic,omitempty"`
	Status     string          `json:"status"`
	LastEvent  time.Time       `json:"last_event"`
	Context    *events.Context `json:"context"`
}

// IterationRecord represents a single completed iteration.
type IterationRecord struct {
	Iteration    int       `json:"iteration"`
	DurationMs   int64     `json:"duration_ms"`
	TaskID       string    `json:"task_id"`
	Passed       bool      `json:"passed"`
	Notes        string    `json:"notes"`
	ReviewCycles int       `json:"review_cycles"`
	FinalVerdict string    `json:"final_verdict"`
	Timestamp    time.Time `json:"timestamp"`
}

// Session represents a Ralph loop session.
type Session struct {
	SessionID   string     `json:"session_id"`
	InstanceID  string     `json:"instance_id"`
	Repo        string     `json:"repo"`
	Epic        string     `json:"epic,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	Iterations  int        `json:"iterations"`
	TasksClosed int        `json:"tasks_closed"`
	PassRate    float64    `json:"pass_rate"`
	EndReason   string     `json:"end_reason,omitempty"`
}

// SessionFilter specifies criteria for querying sessions.
type SessionFilter struct {
	Repo     string     // TODO: implement repo filtering in SQLiteStore.GetSessions
	DateFrom *time.Time // TODO: implement date range filtering in SQLiteStore.GetSessions
	DateTo   *time.Time // TODO: implement date range filtering in SQLiteStore.GetSessions
	Limit    int
	Offset   int
}

// SessionDetail contains a session and all its events.
type SessionDetail struct {
	Session Session        `json:"session"`
	Events  []events.Event `json:"events"`
}

// AggregateStats holds aggregate metrics across all sessions.
type AggregateStats struct {
	TotalSessions    int     `json:"total_sessions"`
	ActiveInstances  int     `json:"active_instances"`
	TotalTasksClosed int     `json:"total_tasks_closed"`
	OverallPassRate  float64 `json:"overall_pass_rate"`
	TotalIterations  int     `json:"total_iterations"`
}

// Store defines the persistence interface for Ralph Hub.
type Store interface {
	SaveEvent(ctx context.Context, event events.Event) error
	GetActiveInstances(ctx context.Context) ([]InstanceState, error)
	GetInstanceHistory(ctx context.Context, instanceID string, limit int) ([]IterationRecord, error)
	GetSessions(ctx context.Context, filter SessionFilter) ([]Session, error)
	GetSessionDetail(ctx context.Context, sessionID string) (*SessionDetail, error)
	GetAggregateStats(ctx context.Context) (*AggregateStats, error)
	EndSession(ctx context.Context, sessionID string, reason string) error
	Close() error
}

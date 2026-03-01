package events

import (
	"errors"
	"fmt"
	"time"
)

// Type represents an event type emitted by the Ralph Loop.
type Type string

const (
	TypeSessionStarted     Type = "session.started"
	TypeSessionEnded       Type = "session.ended"
	TypeIterationStarted   Type = "iteration.started"
	TypeIterationCompleted Type = "iteration.completed"
	TypePhaseChanged       Type = "phase.changed"
	TypeTaskClaimed        Type = "task.claimed"
	TypeTaskClosed         Type = "task.closed"
)

var knownTypes = map[Type]bool{
	TypeSessionStarted:     true,
	TypeSessionEnded:       true,
	TypeIterationStarted:   true,
	TypeIterationCompleted: true,
	TypePhaseChanged:       true,
	TypeTaskClaimed:        true,
	TypeTaskClosed:         true,
}

// Event is the top-level envelope for all Ralph Loop events.
type Event struct {
	EventID    string    `json:"event_id"`
	Type       Type      `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	InstanceID string    `json:"instance_id"`
	Repo       string    `json:"repo"`
	Epic       string    `json:"epic"`
	Data       *Data     `json:"data,omitempty"`
	Context    *Context  `json:"context,omitempty"`
}

// Data carries the event-specific payload fields. Fields are a union
// across all event types; irrelevant fields are left at zero/omitted.
type Data struct {
	Iteration     int    `json:"iteration,omitempty"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	Passed        *bool  `json:"passed,omitempty"`
	Notes         string `json:"notes,omitempty"`
	ReviewCycles  int    `json:"review_cycles,omitempty"`
	Verdict       string `json:"verdict,omitempty"`
	Phase         string `json:"phase,omitempty"`
	FromPhase     string `json:"from_phase,omitempty"`
	ToPhase       string `json:"to_phase,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Description   string `json:"description,omitempty"`
	CommitHash    string `json:"commit_hash,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty"`
}

// Context carries a snapshot of the session state at the time the event
// was emitted, so any single event can reconstruct dashboard state.
type Context struct {
	SessionID        string     `json:"session_id"`
	SessionStart     time.Time  `json:"session_start"`
	MaxIterations    int        `json:"max_iterations"`
	CurrentIteration int        `json:"current_iteration"`
	Status           string     `json:"status"`
	CurrentPhase     string     `json:"current_phase"`
	Analytics        *Analytics `json:"analytics,omitempty"`
}

// Analytics holds aggregated metrics for the session so far.
type Analytics struct {
	PassedCount     int   `json:"passed_count"`
	FailedCount     int   `json:"failed_count"`
	TasksClosed     int   `json:"tasks_closed"`
	InitialReady    int   `json:"initial_ready"`
	CurrentReady    int   `json:"current_ready"`
	AvgDurationMs   int64 `json:"avg_duration_ms"`
	TotalDurationMs int64 `json:"total_duration_ms"`
}

// Validate checks that all required fields are present and the event type
// is one of the known constants.
func (e *Event) Validate() error {
	if e.EventID == "" {
		return errors.New("event_id is required")
	}
	if e.Type == "" {
		return errors.New("type is required")
	}
	if !knownTypes[e.Type] {
		return fmt.Errorf("unknown event type: %q", e.Type)
	}
	if e.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	if e.InstanceID == "" {
		return errors.New("instance_id is required")
	}
	if e.Repo == "" {
		return errors.New("repo is required")
	}
	if e.Context == nil {
		return errors.New("context is required")
	}
	return nil
}

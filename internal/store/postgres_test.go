package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
)

func skipUnlessPostgres(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("RALPH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RALPH_TEST_POSTGRES_DSN not set, skipping postgres tests")
	}
	return dsn
}

func newTestPostgres(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := skipUnlessPostgres(t)
	s, err := NewPostgresStore(dsn)
	if err != nil {
		t.Fatalf("new postgres: %v", err)
	}
	t.Cleanup(func() {
		// Drop tables so each test starts clean.
		s.db.Exec("DROP TABLE IF EXISTS events")
		s.db.Exec("DROP TABLE IF EXISTS instance_state")
		s.Close()
	})
	// Truncate tables for a clean state.
	s.db.Exec("TRUNCATE TABLE events, instance_state")
	return s
}

func TestPostgres_SaveAndGetActiveInstances(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	evt := events.Event{
		EventID:    "evt_1",
		Type:       events.TypeSessionStarted,
		Timestamp:  time.Now().UTC(),
		InstanceID: "my-app/BD-42",
		Repo:       "my-app",
		Epic:       "BD-42",
		Data:       &events.Data{MaxIterations: 50},
		Context: &events.Context{
			SessionID:     "sess_1",
			MaxIterations: 50,
			Status:        "running",
			CurrentPhase:  "planner",
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
	if instances[0].Status != "running" {
		t.Errorf("status = %s, want running", instances[0].Status)
	}
}

func TestPostgres_SessionEndedUpdatesStatus(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	// Start session
	s.SaveEvent(ctx, events.Event{
		EventID:    "evt_1",
		Type:       events.TypeSessionStarted,
		Timestamp:  time.Now().UTC(),
		InstanceID: "my-app/BD-42",
		Repo:       "my-app",
		Context:    &events.Context{SessionID: "sess_1", Status: "running"},
	})

	// End session
	s.SaveEvent(ctx, events.Event{
		EventID:    "evt_2",
		Type:       events.TypeSessionEnded,
		Timestamp:  time.Now().UTC(),
		InstanceID: "my-app/BD-42",
		Repo:       "my-app",
		Data:       &events.Data{Reason: "complete"},
		Context:    &events.Context{SessionID: "sess_1", Status: "running"},
	})

	instances, err := s.GetActiveInstances(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("count = %d, want 0 (ended instances should not appear in active list)", len(instances))
	}
}

func TestPostgres_IterationHistory(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		passed := true
		evt := events.Event{
			EventID:    fmt.Sprintf("evt_%d", i),
			Type:       events.TypeIterationCompleted,
			Timestamp:  time.Now().UTC().Add(time.Duration(i) * time.Second),
			InstanceID: "my-app",
			Repo:       "my-app",
			Data: &events.Data{
				Iteration:    i,
				DurationMs:   30000,
				TaskID:       fmt.Sprintf("BD-%d", i),
				Passed:       &passed,
				Notes:        "done",
				ReviewCycles: 1,
				FinalVerdict: "APPROVED",
			},
			Context: &events.Context{SessionID: "sess_1", Status: "running"},
		}
		if err := s.SaveEvent(ctx, evt); err != nil {
			t.Fatalf("save iteration %d: %v", i, err)
		}
	}

	history, err := s.GetInstanceHistory(ctx, "my-app", 10)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("count = %d, want 3", len(history))
	}
	// Results are ordered DESC, so first is iteration 3
	if history[0].Iteration != 3 {
		t.Errorf("first iteration = %d, want 3", history[0].Iteration)
	}
	if history[0].TaskID != "BD-3" {
		t.Errorf("task_id = %s, want BD-3", history[0].TaskID)
	}
	if !history[0].Passed {
		t.Error("passed = false, want true")
	}
	if history[0].FinalVerdict != "APPROVED" {
		t.Errorf("verdict = %s, want APPROVED", history[0].FinalVerdict)
	}
}

func TestPostgres_IterationHistoryLimit(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		passed := true
		s.SaveEvent(ctx, events.Event{
			EventID:    fmt.Sprintf("evt_%d", i),
			Type:       events.TypeIterationCompleted,
			Timestamp:  time.Now().UTC().Add(time.Duration(i) * time.Second),
			InstanceID: "my-app",
			Repo:       "my-app",
			Data:       &events.Data{Iteration: i, Passed: &passed},
			Context:    &events.Context{SessionID: "sess_1", Status: "running"},
		})
	}

	history, err := s.GetInstanceHistory(ctx, "my-app", 2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(history) != 2 {
		t.Errorf("count = %d, want 2", len(history))
	}
}

func TestPostgres_GetSessions(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	// Start a session
	s.SaveEvent(ctx, events.Event{
		EventID:    "evt_start",
		Type:       events.TypeSessionStarted,
		Timestamp:  time.Now().UTC(),
		InstanceID: "my-app/BD-42",
		Repo:       "my-app",
		Epic:       "BD-42",
		Context: &events.Context{
			SessionID:     "sess_1",
			MaxIterations: 50,
			Status:        "running",
			Analytics:     &events.Analytics{PassedCount: 3, FailedCount: 1, TasksClosed: 3},
		},
	})

	sessions, err := s.GetSessions(ctx, SessionFilter{})
	if err != nil {
		t.Fatalf("get sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("count = %d, want 1", len(sessions))
	}
	if sessions[0].SessionID != "sess_1" {
		t.Errorf("session_id = %s, want sess_1", sessions[0].SessionID)
	}
	if sessions[0].TasksClosed != 3 {
		t.Errorf("tasks_closed = %d, want 3", sessions[0].TasksClosed)
	}
}

func TestPostgres_GetSessionDetail(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	baseCtx := &events.Context{
		SessionID:     "sess_detail",
		MaxIterations: 10,
		Status:        "running",
		Analytics:     &events.Analytics{PassedCount: 1},
	}

	s.SaveEvent(ctx, events.Event{
		EventID: "evt_1", Type: events.TypeSessionStarted,
		Timestamp: time.Now().UTC(), InstanceID: "app", Repo: "app",
		Context: baseCtx,
	})
	passed := true
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_2", Type: events.TypeIterationCompleted,
		Timestamp: time.Now().UTC().Add(time.Second), InstanceID: "app", Repo: "app",
		Data:    &events.Data{Iteration: 1, Passed: &passed},
		Context: baseCtx,
	})

	detail, err := s.GetSessionDetail(ctx, "sess_detail")
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if len(detail.Events) != 2 {
		t.Errorf("events count = %d, want 2", len(detail.Events))
	}
	if detail.Session.SessionID != "sess_detail" {
		t.Errorf("session_id = %s, want sess_detail", detail.Session.SessionID)
	}
}

func TestPostgres_GetSessionDetail_NotFound(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	_, err := s.GetSessionDetail(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestPostgres_GetAggregateStats(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	// Start a session
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_start", Type: events.TypeSessionStarted,
		Timestamp: time.Now().UTC(), InstanceID: "app", Repo: "app",
		Context: &events.Context{SessionID: "sess_1", Status: "running"},
	})

	// Two passing iterations, one failing
	passed := true
	failed := false
	for i, p := range []*bool{&passed, &passed, &failed} {
		s.SaveEvent(ctx, events.Event{
			EventID: fmt.Sprintf("evt_iter_%d", i), Type: events.TypeIterationCompleted,
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Second),
			InstanceID: "app", Repo: "app",
			Data:    &events.Data{Iteration: i + 1, Passed: p},
			Context: &events.Context{SessionID: "sess_1", Status: "running"},
		})
	}

	// One task closed
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_task", Type: events.TypeTaskClosed,
		Timestamp: time.Now().UTC(), InstanceID: "app", Repo: "app",
		Data:    &events.Data{TaskID: "BD-1", CommitHash: "abc123"},
		Context: &events.Context{SessionID: "sess_1", Status: "running"},
	})

	stats, err := s.GetAggregateStats(ctx)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("total_sessions = %d, want 1", stats.TotalSessions)
	}
	if stats.ActiveInstances != 1 {
		t.Errorf("active_instances = %d, want 1", stats.ActiveInstances)
	}
	if stats.TotalIterations != 3 {
		t.Errorf("total_iterations = %d, want 3", stats.TotalIterations)
	}
	if stats.TotalTasksClosed != 1 {
		t.Errorf("total_tasks_closed = %d, want 1", stats.TotalTasksClosed)
	}
	// 2 passed out of 3 total
	expectedRate := 2.0 / 3.0
	if stats.OverallPassRate < expectedRate-0.01 || stats.OverallPassRate > expectedRate+0.01 {
		t.Errorf("pass_rate = %f, want ~%f", stats.OverallPassRate, expectedRate)
	}
}

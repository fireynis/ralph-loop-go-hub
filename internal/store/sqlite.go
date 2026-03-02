package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite via modernc.org/sqlite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database at path and runs migrations.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.reconcileInstanceState(); err != nil {
		db.Close()
		return nil, fmt.Errorf("reconcile: %w", err)
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

// reconcileInstanceState fixes instances stuck as "running" when their
// latest session has already ended (e.g. from events ingested before the
// status-update logic existed).
func (s *SQLiteStore) reconcileInstanceState() error {
	_, err := s.db.Exec(`
		UPDATE instance_state SET status = 'ended'
		WHERE status = 'running'
		AND instance_id IN (
			SELECT e.instance_id FROM events e
			WHERE e.type = 'session.ended'
			AND NOT EXISTS (
				SELECT 1 FROM events e2
				WHERE e2.instance_id = e.instance_id
				AND e2.type = 'session.started'
				AND e2.timestamp > e.timestamp
			)
		)`)
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
		evt.EventID, string(evt.Type), evt.Timestamp.UTC().Format(time.RFC3339Nano),
		evt.InstanceID, evt.Repo, evt.Epic, string(dataJSON), string(ctxJSON))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	status := "running"
	if evt.Context != nil {
		status = evt.Context.Status
	}
	if evt.Type == events.TypeSessionEnded {
		status = "ended"
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO instance_state (instance_id, repo, epic, status, last_event, context_json)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(instance_id) DO UPDATE SET
		   status = excluded.status,
		   last_event = excluded.last_event,
		   context_json = excluded.context_json`,
		evt.InstanceID, evt.Repo, evt.Epic, status,
		evt.Timestamp.UTC().Format(time.RFC3339Nano), string(ctxJSON))
	if err != nil {
		return fmt.Errorf("upsert instance: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetActiveInstances(ctx context.Context) ([]InstanceState, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT instance_id, repo, epic, status, last_event, context_json
		 FROM instance_state WHERE status = 'running' ORDER BY last_event DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []InstanceState
	for rows.Next() {
		var inst InstanceState
		var ctxJSON string
		var lastEvent string
		if err := rows.Scan(&inst.InstanceID, &inst.Repo, &inst.Epic, &inst.Status, &lastEvent, &ctxJSON); err != nil {
			return nil, err
		}
		inst.LastEvent, err = time.Parse(time.RFC3339Nano, lastEvent)
		if err != nil {
			return nil, fmt.Errorf("parse last_event time: %w", err)
		}
		var evtCtx events.Context
		if err := json.Unmarshal([]byte(ctxJSON), &evtCtx); err != nil {
			return nil, fmt.Errorf("unmarshal context: %w", err)
		}
		inst.Context = &evtCtx
		instances = append(instances, inst)
	}
	return instances, rows.Err()
}

func (s *SQLiteStore) GetInstanceHistory(ctx context.Context, instanceID string, limit int) ([]IterationRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data_json, timestamp FROM events
		 WHERE instance_id = ? AND type = ?
		 ORDER BY timestamp DESC LIMIT ?`,
		instanceID, string(events.TypeIterationCompleted), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []IterationRecord
	for rows.Next() {
		var dataJSON string
		var tsStr string
		if err := rows.Scan(&dataJSON, &tsStr); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339Nano, tsStr)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp: %w", err)
		}

		var data events.Data
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}

		rec := IterationRecord{
			Timestamp:    ts,
			Iteration:    data.Iteration,
			DurationMs:   data.DurationMs,
			TaskID:       data.TaskID,
			Notes:        data.Notes,
			ReviewCycles: data.ReviewCycles,
			FinalVerdict: data.FinalVerdict,
		}
		if data.Passed != nil {
			rec.Passed = *data.Passed
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
		WHERE e_start.type = 'session.started'`

	args := []any{}
	if filter.InstanceID != "" {
		query += ` AND e_start.instance_id = ?`
		args = append(args, filter.InstanceID)
	}

	query += ` ORDER BY e_start.timestamp DESC LIMIT ? OFFSET ?`

	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}
	args = append(args, limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var ctxJSON string
		var endedAt sql.NullString
		var endDataJSON sql.NullString

		if err := rows.Scan(&ctxJSON, &sess.InstanceID, &sess.Repo, &sess.Epic, &sess.StartedAt, &endedAt, &endDataJSON); err != nil {
			return nil, err
		}

		var evtCtx events.Context
		if err := json.Unmarshal([]byte(ctxJSON), &evtCtx); err != nil {
			return nil, fmt.Errorf("unmarshal context: %w", err)
		}
		sess.SessionID = evtCtx.SessionID
		sess.Iterations = evtCtx.CurrentIteration
		if evtCtx.Analytics != nil {
			sess.TasksClosed = evtCtx.Analytics.TasksClosed
			total := evtCtx.Analytics.PassedCount + evtCtx.Analytics.FailedCount
			if total > 0 {
				sess.PassRate = float64(evtCtx.Analytics.PassedCount) / float64(total)
			}
		}

		if endedAt.Valid && endedAt.String != "" {
			t, err := time.Parse(time.RFC3339Nano, endedAt.String)
			if err == nil {
				sess.EndedAt = &t
			}
		}
		if endDataJSON.Valid {
			var endData events.Data
			if err := json.Unmarshal([]byte(endDataJSON.String), &endData); err != nil {
				return nil, fmt.Errorf("unmarshal end data: %w", err)
			}
			sess.EndReason = endData.Reason
		}

		sessions = append(sessions, sess)
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
		var tsStr string
		if err := rows.Scan(&evt.EventID, &evt.Type, &tsStr, &evt.InstanceID, &evt.Repo, &evt.Epic, &dataJSON, &ctxJSON); err != nil {
			return nil, err
		}
		evt.Timestamp, err = time.Parse(time.RFC3339Nano, tsStr)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp: %w", err)
		}
		var data events.Data
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}
		evt.Data = &data
		var evtCtx events.Context
		if err := json.Unmarshal([]byte(ctxJSON), &evtCtx); err != nil {
			return nil, fmt.Errorf("unmarshal context: %w", err)
		}
		evt.Context = &evtCtx
		detail.Events = append(detail.Events, evt)
	}

	if len(detail.Events) == 0 {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	first := detail.Events[0]
	last := detail.Events[len(detail.Events)-1]
	detail.Session = Session{
		SessionID:  sessionID,
		InstanceID: first.InstanceID,
		Repo:       first.Repo,
		Epic:       first.Epic,
		StartedAt:  first.Timestamp,
	}
	if last.Context != nil {
		detail.Session.Iterations = last.Context.CurrentIteration
		if last.Context.Analytics != nil {
			detail.Session.TasksClosed = last.Context.Analytics.TasksClosed
			total := last.Context.Analytics.PassedCount + last.Context.Analytics.FailedCount
			if total > 0 {
				detail.Session.PassRate = float64(last.Context.Analytics.PassedCount) / float64(total)
			}
		}
	}
	if last.Type == events.TypeSessionEnded {
		detail.Session.EndedAt = &last.Timestamp
		if last.Data != nil {
			detail.Session.EndReason = last.Data.Reason
		}
	}

	return detail, rows.Err()
}

func (s *SQLiteStore) GetAggregateStats(ctx context.Context) (*AggregateStats, error) {
	stats := &AggregateStats{}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'session.started'`).Scan(&stats.TotalSessions); err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM instance_state WHERE status = 'running'`).Scan(&stats.ActiveInstances); err != nil {
		return nil, fmt.Errorf("count active instances: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'`).Scan(&stats.TotalIterations); err != nil {
		return nil, fmt.Errorf("count iterations: %w", err)
	}

	var passed, failed int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'
		 AND json_extract(data_json, '$.passed') = true`).Scan(&passed); err != nil {
		return nil, fmt.Errorf("count passed: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'
		 AND json_extract(data_json, '$.passed') = false`).Scan(&failed); err != nil {
		return nil, fmt.Errorf("count failed: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'task.closed'`).Scan(&stats.TotalTasksClosed); err != nil {
		return nil, fmt.Errorf("count tasks closed: %w", err)
	}

	if passed+failed > 0 {
		stats.OverallPassRate = float64(passed) / float64(passed+failed)
	}

	return stats, nil
}

func (s *SQLiteStore) EndSession(ctx context.Context, sessionID string, reason string) error {
	var instanceID, repo, epic, ctxJSON, lastTS string
	err := s.db.QueryRowContext(ctx,
		`SELECT instance_id, repo, epic, context_json, timestamp FROM events
		 WHERE json_extract(context_json, '$.session_id') = ?
		 ORDER BY timestamp DESC LIMIT 1`, sessionID).Scan(&instanceID, &repo, &epic, &ctxJSON, &lastTS)
	if err != nil {
		return ErrSessionNotFound
	}

	var endCount int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events
		 WHERE json_extract(context_json, '$.session_id') = ? AND type = ?`,
		sessionID, string(events.TypeSessionEnded)).Scan(&endCount)
	if endCount > 0 {
		return ErrSessionAlreadyEnded
	}

	var evtCtx events.Context
	if err := json.Unmarshal([]byte(ctxJSON), &evtCtx); err != nil {
		return fmt.Errorf("unmarshal context: %w", err)
	}
	evtCtx.Status = "ended"

	if reason == "" {
		reason = "manual_close"
	}

	// Use whichever is later: now or the last event's timestamp + 1ms,
	// so the synthesized event is guaranteed to sort after all existing events.
	now := time.Now().UTC()
	lastTime, err := time.Parse(time.RFC3339Nano, lastTS)
	if err == nil && !lastTime.Before(now) {
		now = lastTime.Add(time.Millisecond)
	}

	evt := events.Event{
		EventID:    uuid.NewString(),
		Type:       events.TypeSessionEnded,
		Timestamp:  now,
		InstanceID: instanceID,
		Repo:       repo,
		Epic:       epic,
		Data:       &events.Data{Reason: reason},
		Context:    &evtCtx,
	}

	return s.SaveEvent(ctx, evt)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

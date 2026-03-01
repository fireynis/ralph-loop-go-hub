package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fireynis/ralph-hub/internal/events"
	_ "github.com/lib/pq"
)

// PostgresStore implements Store using PostgreSQL via lib/pq.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore opens a PostgreSQL connection using the provided DSN,
// verifies connectivity, and runs schema migrations.
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &PostgresStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *PostgresStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			instance_id TEXT NOT NULL,
			repo TEXT NOT NULL,
			epic TEXT DEFAULT '',
			data_json JSONB NOT NULL,
			context_json JSONB NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_events_instance ON events(instance_id);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

		CREATE TABLE IF NOT EXISTS instance_state (
			instance_id TEXT PRIMARY KEY,
			repo TEXT NOT NULL,
			epic TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'running',
			last_event TIMESTAMPTZ NOT NULL,
			context_json JSONB NOT NULL
		);
	`)
	return err
}

func (s *PostgresStore) SaveEvent(ctx context.Context, evt events.Event) error {
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
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		evt.EventID, string(evt.Type), evt.Timestamp.UTC(),
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
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT(instance_id) DO UPDATE SET
		   status = EXCLUDED.status,
		   last_event = EXCLUDED.last_event,
		   context_json = EXCLUDED.context_json`,
		evt.InstanceID, evt.Repo, evt.Epic, status,
		evt.Timestamp.UTC(), string(ctxJSON))
	if err != nil {
		return fmt.Errorf("upsert instance: %w", err)
	}

	return tx.Commit()
}

func (s *PostgresStore) GetActiveInstances(ctx context.Context) ([]InstanceState, error) {
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
		if err := rows.Scan(&inst.InstanceID, &inst.Repo, &inst.Epic, &inst.Status, &inst.LastEvent, &ctxJSON); err != nil {
			return nil, err
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

func (s *PostgresStore) GetInstanceHistory(ctx context.Context, instanceID string, limit int) ([]IterationRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data_json, timestamp FROM events
		 WHERE instance_id = $1 AND type = $2
		 ORDER BY timestamp DESC LIMIT $3`,
		instanceID, string(events.TypeIterationCompleted), limit)
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

func (s *PostgresStore) GetSessions(ctx context.Context, filter SessionFilter) ([]Session, error) {
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
			e_start.context_json->>'session_id' = e_end.context_json->>'session_id'
			AND e_end.type = 'session.ended'
		WHERE e_start.type = 'session.started'
		ORDER BY e_start.timestamp DESC
		LIMIT $1 OFFSET $2`

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
		var sess Session
		var ctxJSON string
		var endedAt *time.Time
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

		if endedAt != nil {
			sess.EndedAt = endedAt
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

func (s *PostgresStore) GetSessionDetail(ctx context.Context, sessionID string) (*SessionDetail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, timestamp, instance_id, repo, epic, data_json, context_json
		 FROM events
		 WHERE context_json->>'session_id' = $1
		 ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	detail := &SessionDetail{}
	for rows.Next() {
		var evt events.Event
		var dataJSON, ctxJSON string
		if err := rows.Scan(&evt.EventID, &evt.Type, &evt.Timestamp, &evt.InstanceID, &evt.Repo, &evt.Epic, &dataJSON, &ctxJSON); err != nil {
			return nil, err
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
	}

	return detail, rows.Err()
}

func (s *PostgresStore) GetAggregateStats(ctx context.Context) (*AggregateStats, error) {
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
		 AND (data_json->>'passed')::boolean = true`).Scan(&passed); err != nil {
		return nil, fmt.Errorf("count passed: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE type = 'iteration.completed'
		 AND (data_json->>'passed')::boolean = false`).Scan(&failed); err != nil {
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

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

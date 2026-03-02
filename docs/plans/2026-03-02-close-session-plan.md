# Close Session Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow manually closing stale sessions via API + dashboard button by synthesizing a `session.ended` event.

**Architecture:** `EndSession` store method looks up the session's last event, builds a synthetic `session.ended` event, and delegates to `SaveEvent`. A new unauthenticated `POST /api/v1/sessions/{id}/end` handler calls this. The frontend adds a button on the session detail page.

**Tech Stack:** Go 1.25 (stdlib testing), Next.js 16 (React 19), SQLite + Postgres stores, `google/uuid` (already indirect dep).

---

### Task 1: Add `EndSession` to the Store Interface + Sentinel Errors

**Files:**
- Modify: `internal/store/store.go:70-79`

**Step 1: Add sentinel errors and `EndSession` to the interface**

Add above the `Store` interface:

```go
import "errors"

var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionAlreadyEnded = errors.New("session already ended")
)
```

Add to the `Store` interface:

```go
EndSession(ctx context.Context, sessionID string, reason string) error
```

**Step 2: Verify it compiles (it won't — implementations missing)**

Run: `go build ./...`
Expected: compile errors for SQLiteStore and PostgresStore not implementing EndSession.

**Step 3: Commit**

```bash
git add internal/store/store.go
git commit -m "feat: add EndSession to Store interface with sentinel errors"
```

---

### Task 2: Implement `EndSession` for SQLiteStore (TDD)

**Files:**
- Test: `internal/store/sqlite_test.go`
- Modify: `internal/store/sqlite.go`

**Step 1: Promote `google/uuid` to a direct dependency**

Run: `go get github.com/google/uuid`

This is already an indirect dep (via modernc.org/sqlite). We need it directly for generating synthetic event IDs.

**Step 2: Write the failing tests**

Append to `internal/store/sqlite_test.go`:

```go
func TestSQLite_EndSession(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	// Start a session with some iterations.
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_start", Type: events.TypeSessionStarted,
		Timestamp: time.Now().UTC(), InstanceID: "app/BD-1", Repo: "app", Epic: "BD-1",
		Context: &events.Context{
			SessionID: "sess_end_test", MaxIterations: 10, Status: "running",
			Analytics: &events.Analytics{PassedCount: 2, TasksClosed: 1},
		},
	})
	passed := true
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_iter", Type: events.TypeIterationCompleted,
		Timestamp: time.Now().UTC().Add(time.Second), InstanceID: "app/BD-1", Repo: "app", Epic: "BD-1",
		Data: &events.Data{Iteration: 1, Passed: &passed},
		Context: &events.Context{
			SessionID: "sess_end_test", MaxIterations: 10, Status: "running", CurrentIteration: 1,
			Analytics: &events.Analytics{PassedCount: 2, TasksClosed: 1},
		},
	})

	// End the session manually.
	if err := s.EndSession(ctx, "sess_end_test", "stale"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	// Session should now show as ended.
	detail, err := s.GetSessionDetail(ctx, "sess_end_test")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if detail.Session.EndedAt == nil {
		t.Fatal("expected session to have EndedAt set")
	}
	if detail.Session.EndReason != "stale" {
		t.Errorf("end_reason = %q, want %q", detail.Session.EndReason, "stale")
	}

	// The last event should be session.ended.
	last := detail.Events[len(detail.Events)-1]
	if last.Type != events.TypeSessionEnded {
		t.Errorf("last event type = %s, want session.ended", last.Type)
	}
	if last.Data.Reason != "stale" {
		t.Errorf("last event reason = %q, want %q", last.Data.Reason, "stale")
	}

	// Instance should no longer be active.
	instances, err := s.GetActiveInstances(ctx)
	if err != nil {
		t.Fatalf("GetActiveInstances: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("active instances = %d, want 0", len(instances))
	}
}

func TestSQLite_EndSession_NotFound(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	err := s.EndSession(ctx, "nonexistent", "stale")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestSQLite_EndSession_AlreadyEnded(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	s.SaveEvent(ctx, events.Event{
		EventID: "evt_start", Type: events.TypeSessionStarted,
		Timestamp: time.Now().UTC(), InstanceID: "app", Repo: "app",
		Context: &events.Context{SessionID: "sess_already", Status: "running"},
	})
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_end", Type: events.TypeSessionEnded,
		Timestamp: time.Now().UTC().Add(time.Second), InstanceID: "app", Repo: "app",
		Data: &events.Data{Reason: "complete"},
		Context: &events.Context{SessionID: "sess_already", Status: "running"},
	})

	err := s.EndSession(ctx, "sess_already", "stale")
	if !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Errorf("expected ErrSessionAlreadyEnded, got %v", err)
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/store/ -run TestSQLite_EndSession -v`
Expected: compile error — `EndSession` not defined.

**Step 4: Implement `EndSession` on SQLiteStore**

Add to `internal/store/sqlite.go`. Add `"github.com/google/uuid"` to imports:

```go
func (s *SQLiteStore) EndSession(ctx context.Context, sessionID string, reason string) error {
	// Look up the most recent event for this session.
	var instanceID, repo, epic, ctxJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT instance_id, repo, epic, context_json FROM events
		 WHERE json_extract(context_json, '$.session_id') = ?
		 ORDER BY timestamp DESC LIMIT 1`, sessionID).Scan(&instanceID, &repo, &epic, &ctxJSON)
	if err != nil {
		return ErrSessionNotFound
	}

	// Check if session is already ended.
	var endCount int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events
		 WHERE json_extract(context_json, '$.session_id') = ? AND type = ?`,
		sessionID, string(events.TypeSessionEnded)).Scan(&endCount)
	if endCount > 0 {
		return ErrSessionAlreadyEnded
	}

	// Reconstruct the context from the last event.
	var evtCtx events.Context
	if err := json.Unmarshal([]byte(ctxJSON), &evtCtx); err != nil {
		return fmt.Errorf("unmarshal context: %w", err)
	}
	evtCtx.Status = "ended"

	if reason == "" {
		reason = "manual_close"
	}

	evt := events.Event{
		EventID:    uuid.NewString(),
		Type:       events.TypeSessionEnded,
		Timestamp:  time.Now().UTC(),
		InstanceID: instanceID,
		Repo:       repo,
		Epic:       epic,
		Data:       &events.Data{Reason: reason},
		Context:    &evtCtx,
	}

	return s.SaveEvent(ctx, evt)
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/ -run TestSQLite_EndSession -v`
Expected: all 3 tests PASS.

**Step 6: Run full test suite**

Run: `go test ./...`
Expected: compile error — PostgresStore doesn't implement EndSession yet.

**Step 7: Commit**

```bash
git add internal/store/sqlite.go internal/store/sqlite_test.go go.mod go.sum
git commit -m "feat: implement EndSession for SQLiteStore"
```

---

### Task 3: Implement `EndSession` for PostgresStore

**Files:**
- Test: `internal/store/postgres_test.go`
- Modify: `internal/store/postgres.go`

**Step 1: Write the failing tests**

Append to `internal/store/postgres_test.go`:

```go
func TestPostgres_EndSession(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	s.SaveEvent(ctx, events.Event{
		EventID: "evt_start", Type: events.TypeSessionStarted,
		Timestamp: time.Now().UTC(), InstanceID: "app/BD-1", Repo: "app", Epic: "BD-1",
		Context: &events.Context{
			SessionID: "sess_end_test", MaxIterations: 10, Status: "running",
			Analytics: &events.Analytics{PassedCount: 2, TasksClosed: 1},
		},
	})
	passed := true
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_iter", Type: events.TypeIterationCompleted,
		Timestamp: time.Now().UTC().Add(time.Second), InstanceID: "app/BD-1", Repo: "app", Epic: "BD-1",
		Data: &events.Data{Iteration: 1, Passed: &passed},
		Context: &events.Context{
			SessionID: "sess_end_test", MaxIterations: 10, Status: "running", CurrentIteration: 1,
			Analytics: &events.Analytics{PassedCount: 2, TasksClosed: 1},
		},
	})

	if err := s.EndSession(ctx, "sess_end_test", "stale"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	detail, err := s.GetSessionDetail(ctx, "sess_end_test")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if detail.Session.EndedAt == nil {
		t.Fatal("expected session to have EndedAt set")
	}
	if detail.Session.EndReason != "stale" {
		t.Errorf("end_reason = %q, want %q", detail.Session.EndReason, "stale")
	}

	last := detail.Events[len(detail.Events)-1]
	if last.Type != events.TypeSessionEnded {
		t.Errorf("last event type = %s, want session.ended", last.Type)
	}

	instances, err := s.GetActiveInstances(ctx)
	if err != nil {
		t.Fatalf("GetActiveInstances: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("active instances = %d, want 0", len(instances))
	}
}

func TestPostgres_EndSession_NotFound(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	err := s.EndSession(ctx, "nonexistent", "stale")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestPostgres_EndSession_AlreadyEnded(t *testing.T) {
	s := newTestPostgres(t)
	ctx := context.Background()

	s.SaveEvent(ctx, events.Event{
		EventID: "evt_start", Type: events.TypeSessionStarted,
		Timestamp: time.Now().UTC(), InstanceID: "app", Repo: "app",
		Context: &events.Context{SessionID: "sess_already", Status: "running"},
	})
	s.SaveEvent(ctx, events.Event{
		EventID: "evt_end", Type: events.TypeSessionEnded,
		Timestamp: time.Now().UTC().Add(time.Second), InstanceID: "app", Repo: "app",
		Data: &events.Data{Reason: "complete"},
		Context: &events.Context{SessionID: "sess_already", Status: "running"},
	})

	err := s.EndSession(ctx, "sess_already", "stale")
	if !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Errorf("expected ErrSessionAlreadyEnded, got %v", err)
	}
}
```

**Step 2: Implement `EndSession` on PostgresStore**

Add to `internal/store/postgres.go`. Add `"github.com/google/uuid"` to imports:

```go
func (s *PostgresStore) EndSession(ctx context.Context, sessionID string, reason string) error {
	var instanceID, repo, epic, ctxJSON string
	err := s.db.QueryRowContext(ctx,
		`SELECT instance_id, repo, epic, context_json FROM events
		 WHERE context_json->>'session_id' = $1
		 ORDER BY timestamp DESC LIMIT 1`, sessionID).Scan(&instanceID, &repo, &epic, &ctxJSON)
	if err != nil {
		return ErrSessionNotFound
	}

	var endCount int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events
		 WHERE context_json->>'session_id' = $1 AND type = $2`,
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

	evt := events.Event{
		EventID:    uuid.NewString(),
		Type:       events.TypeSessionEnded,
		Timestamp:  time.Now().UTC(),
		InstanceID: instanceID,
		Repo:       repo,
		Epic:       epic,
		Data:       &events.Data{Reason: reason},
		Context:    &evtCtx,
	}

	return s.SaveEvent(ctx, evt)
}
```

**Step 3: Run full test suite**

Run: `go test ./...`
Expected: all tests PASS (Postgres tests skip unless `RALPH_TEST_POSTGRES_DSN` is set).

**Step 4: Commit**

```bash
git add internal/store/postgres.go internal/store/postgres_test.go
git commit -m "feat: implement EndSession for PostgresStore"
```

---

### Task 4: Add the API Handler + Route (TDD)

**Files:**
- Test: `internal/server/server_test.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/server.go:38-63`

**Step 1: Write the failing tests**

Append to `internal/server/server_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestEndSession -v`
Expected: FAIL — 404 on the new route.

**Step 3: Add the handler to `handlers.go`**

Add to `internal/server/handlers.go`. Add `"errors"` and `"github.com/fireynis/ralph-hub/internal/store"` to the import (store is already imported):

```go
// handleEndSession manually closes a running session by synthesizing
// a session.ended event.
func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "session id is required")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	// Body is optional; ignore decode errors for empty body.
	json.NewDecoder(r.Body).Decode(&body)

	if err := s.store.EndSession(r.Context(), id, body.Reason); err != nil {
		if errors.Is(err, store.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, store.ErrSessionAlreadyEnded) {
			writeError(w, http.StatusConflict, "session already ended")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to end session")
		return
	}

	// Return the updated session detail.
	detail, err := s.store.GetSessionDetail(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session ended but failed to fetch detail")
		return
	}

	writeJSON(w, http.StatusOK, detail)
}
```

**Step 4: Register the route in `server.go`**

Add after the `GET /api/v1/sessions/{id}` line in `Handler()`:

```go
mux.HandleFunc("POST /api/v1/sessions/{id}/end", s.handleEndSession)
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -run TestEndSession -v`
Expected: all 3 tests PASS.

**Step 6: Run full test suite**

Run: `go test ./...`
Expected: all tests PASS.

**Step 7: Commit**

```bash
git add internal/server/handlers.go internal/server/server.go internal/server/server_test.go
git commit -m "feat: add POST /api/v1/sessions/{id}/end handler"
```

---

### Task 5: Add "Close Session" Button to Frontend

**Files:**
- Modify: `web/src/app/sessions/[id]/page.tsx`

**Step 1: Add close session handler and button**

In the `SessionDetailPage` component, add state for the close operation. After the existing `useState` declarations (around line 98), add:

```tsx
const [closing, setClosing] = useState(false);
```

Add a `closeSession` function before the loading/error early returns:

```tsx
async function closeSession() {
  if (!confirm('Are you sure you want to close this session?')) return;
  setClosing(true);
  try {
    const res = await fetch(
      `${getApiBase()}/api/v1/sessions/${params.id}/end`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason: 'manual_close' }),
      }
    );
    if (!res.ok) {
      const data = await res.json();
      throw new Error(data.error || `API error: ${res.status}`);
    }
    const data: SessionDetailResponse = await res.json();
    setSession(data.session);
    setEvents(data.events);
  } catch (err) {
    alert(err instanceof Error ? err.message : 'Failed to close session');
  } finally {
    setClosing(false);
  }
}
```

**Step 2: Add the button to the header area**

In the JSX, after the header `<div>` containing the repo name and session ID (around line 166), add the close button. Replace the header block so it includes a flex container with the button:

Change the header section (the outer `<div>` containing `<h1>`) to:

```tsx
{/* Header */}
<div className="flex items-start justify-between">
  <div>
    <h1 className="text-3xl font-bold tracking-tight text-white">
      {session.repo}
    </h1>
    <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-gray-400">
      {session.epic && (
        <span className="rounded-full bg-gray-800 px-3 py-0.5 text-gray-300">
          {session.epic}
        </span>
      )}
      <span className="font-mono text-xs text-gray-500">
        {session.session_id}
      </span>
    </div>
  </div>
  {!session.ended_at && (
    <button
      onClick={closeSession}
      disabled={closing}
      className="rounded-lg border border-red-700 bg-red-900/30 px-4 py-2 text-sm font-medium text-red-300 hover:bg-red-900/50 disabled:opacity-50"
    >
      {closing ? 'Closing...' : 'Close Session'}
    </button>
  )}
</div>
```

**Step 3: Verify build**

Run: `cd web && npm run build`
Expected: build succeeds with no type errors.

**Step 4: Commit**

```bash
git add web/src/app/sessions/\[id\]/page.tsx
git commit -m "feat: add Close Session button to session detail page"
```

---

### Task 6: Final Verification

**Step 1: Run full Go test suite**

Run: `go test ./...`
Expected: all tests PASS.

**Step 2: Run frontend build**

Run: `cd web && npm run build`
Expected: clean build, no errors.

**Step 3: Run frontend lint**

Run: `cd web && npm run lint`
Expected: no lint errors.

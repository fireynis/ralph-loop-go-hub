# Close Session Feature Design

## Problem

Sessions are event-driven: they only end when a Ralph Loop instance sends a `session.ended` event. If a Loop crashes or is killed without sending that event, the session stays "Running" forever with no way to close it.

## Approach: Synthetic Event Injection

Add an `EndSession` store method that looks up the session's latest event context, synthesizes a `session.ended` event, and persists it via the existing `SaveEvent` flow. This keeps the event-sourced data model intact — every session end is a real event in the timeline, and existing read queries work unchanged.

## Components

### 1. Store — `EndSession(ctx, sessionID, reason) error`

- Query the events table for the most recent event matching `session_id` to get instance_id, repo, epic, and context
- Return an error if the session doesn't exist or is already ended
- Build a synthetic `session.ended` event with a generated UUID, current timestamp, and the provided reason (default: "manual_close")
- Call the existing `SaveEvent` to persist and update `instance_state`

Both `SQLiteStore` and `PostgresStore` get this method. The Store interface gains `EndSession`.

### 2. API — `POST /api/v1/sessions/{id}/end`

- No auth required (matches read endpoints)
- Optional JSON body: `{"reason": "stale"}` — defaults to "manual_close" if empty
- On success: returns 200 with the updated session detail
- On not found: 404
- On already ended: 409 Conflict

### 3. WebSocket Broadcast

Automatic — `SaveEvent` already broadcasts to the hub and dispatches webhooks. No additional work needed.

### 4. Frontend — Close Session Button

- Session detail page (`/sessions/[id]`): add a "Close Session" button, visible only when `ended_at` is null
- On click: confirm dialog, then POST to the endpoint
- On success: refetch session data (the timeline will show the synthetic event)

### 5. Tests

- Go unit tests for `EndSession` in both SQLite and Postgres stores
- Handler test for the new endpoint (success, not found, already ended)

# CLAUDE.md

Project-specific conventions for AI coding assistants working on ralph-hub.

## Overview

ralph-hub is a Go API server + Next.js dashboard monorepo for monitoring Ralph Loop instances. It receives events via REST API, stores them, broadcasts via WebSocket, and dispatches outbound webhooks.

- **Module path**: `github.com/fireynis/ralph-hub`
- **Go version**: 1.25.0
- **Domain**: `ralph-hub.home.arpa` (deployed via Docker + Traefik)

## Project Structure

```
cmd/hub/main.go           # Entry point: config, store, hub, dispatcher, HTTP server
internal/
  config/                 # YAML config loading with defaults
  events/                 # Event types (Type constants), Event struct, validation
  server/                 # HTTP server, routes (Go 1.22+ patterns), handlers, middleware
  store/                  # Store interface + SQLite and Postgres implementations
  webhook/                # Outbound webhook dispatcher (async, retries with backoff)
  ws/                     # WebSocket hub (fan-out broadcast to browser clients)
web/                      # Next.js 16 dashboard (App Router)
  src/app/                # Pages: /, /instances/[id], /sessions, /sessions/[id], /settings
  src/components/         # Reusable UI components
  src/hooks/              # Custom hooks (useWebSocket, etc.)
  src/lib/                # API client utilities
  src/store/              # Zustand state stores
```

## Build Commands

```bash
# Go
go build ./cmd/hub/       # Build the server binary
go test ./...             # Run all Go tests
make build                # Same as go build
make lint                 # golangci-lint run
make run                  # Build + run

# Frontend
cd web && npm install     # Install deps
cd web && npm run dev     # Dev server (port 3000)
cd web && npm run build   # Production build (also type-checks)
cd web && npm run lint    # ESLint
```

## Code Conventions

### Go

- **Testing**: Use stdlib `testing` package only. No testify or other assertion libraries.
- **Routing**: Go 1.22+ method-based routing patterns (e.g., `mux.HandleFunc("GET /api/v1/instances", handler)`).
- **Store pattern**: All persistence goes through the `store.Store` interface. Two implementations: `SQLiteStore` and `PostgresStore`. New queries must be added to the interface and both implementations.
- **Dependency injection**: Components are wired in `cmd/hub/main.go` and passed to `server.New()`. No global state.
- **Error handling**: Handlers use `writeJSON` / `writeError` helpers. Store errors surface as HTTP 500. Validation errors as HTTP 400.
- **Middleware**: Auth and CORS are HTTP middleware wrapping the mux. Auth uses Bearer token matching against configured API keys.
- **WebSocket**: Hub uses channel-per-client with non-blocking sends (full channels are skipped). No gorilla-style pump needed -- the server manages read/write loops directly.
- **Webhooks**: Async dispatch in goroutines. Exponential backoff retries (3 attempts). Never blocks event ingestion.

### Frontend (Next.js)

- **Framework**: Next.js 16 with App Router.
- **Styling**: Tailwind CSS v4, dark theme.
- **State management**: Zustand for client state (WebSocket-driven updates).
- **Charts**: Recharts v3 for data visualization.
- **React version**: React 19.

### Test Patterns

- **Go unit tests**: Use in-memory SQLite (`:memory:`) for store tests. No external dependencies needed.
- **Postgres tests**: Skip behind an environment variable check (require `RALPH_TEST_POSTGRES_DSN`).
- **Test files**: Colocated with source as `*_test.go`.

## Key Interfaces

### Store (`internal/store/store.go`)

```go
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

Both `SQLiteStore` and `PostgresStore` implement this. When adding new query methods, update the interface and both implementations.

### Hub (`internal/ws/hub.go`)

WebSocket fan-out manager. Methods: `Register(ch)`, `Unregister(ch)`, `Broadcast(data)`, `ClientCount()`.

### Dispatcher (`internal/webhook/dispatcher.go`)

Outbound webhook delivery. Method: `Dispatch(evt)` -- fires matching webhooks asynchronously.

## API Routes

All routes are registered in `internal/server/server.go` via `Handler()`:

- `GET /healthz` -- health check (no auth)
- `POST /api/v1/events` -- event ingestion (auth required)
- `GET /api/v1/instances` -- list active instances
- `GET /api/v1/instances/{id}/history` -- instance iteration history
- `GET /api/v1/sessions` -- list sessions (paginated)
- `GET /api/v1/sessions/{id}` -- session detail
- `GET /api/v1/stats` -- aggregate stats
- `GET /api/v1/ws` -- WebSocket endpoint

## Dependencies

### Go (notable)

- `gorilla/websocket` -- WebSocket upgrades
- `lib/pq` -- Postgres driver
- `modernc.org/sqlite` -- Pure-Go SQLite (no CGO)
- `gopkg.in/yaml.v3` -- YAML config parsing

### Frontend (notable)

- `next` 16.1.6, `react` 19.2.3
- `recharts` ^3.7.0
- `zustand` ^5.0.11
- `tailwindcss` ^4

# Ralph Hub

Ralph Hub is the centralized event hub and real-time monitoring dashboard for [Ralph Loop](https://github.com/fireynis/ralph-loop-tui) instances. Ralph Loop is an autonomous AI agent TUI that works through project issues in unattended development cycles. Ralph Hub collects structured telemetry from running instances, stores it, broadcasts live updates over WebSocket to a dashboard, and dispatches configurable outbound webhooks.

## Architecture

```
                        +-----------------+
Ralph Instance A ──┐    |                 |    ┌── Next.js Dashboard (browser)
Ralph Instance B ──┼──> |   Go API Hub    | <──┼── Next.js Dashboard (browser)
Ralph Instance C ──┘    |   (port 8080)   |    └── Next.js Dashboard (browser)
   POST /api/v1/events  +--------+--------+       GET /api/v1/ws (WebSocket)
                                 |
                       ┌─────────┴─────────┐
                       |                   |
                 SQLite or Postgres   Webhook Dispatcher
                   (persistence)       (async, retries)
                                       |    |    |
                                     Slack Discord Custom
```

The Go server (`cmd/hub/main.go`) wires together four components via dependency injection -- no global state:

| Component | Package | Purpose |
|-----------|---------|---------|
| **Store** | `internal/store` | Persistence layer (SQLite default, Postgres optional) |
| **Hub** | `internal/ws` | WebSocket fan-out to connected browser clients |
| **Dispatcher** | `internal/webhook` | Outbound webhook delivery with exponential backoff retries |
| **Server** | `internal/server` | HTTP routes, Bearer token auth middleware, CORS |

The Next.js frontend (`web/`) connects via WebSocket for live event streaming and fetches historical data through the query endpoints.

## Quickstart

### Docker (recommended)

```bash
cp config.example.yaml config.yaml   # edit API keys, storage, webhooks
docker compose up -d
```

The dashboard is available at `http://ralph-hub.home.arpa` (requires Traefik and DNS) or you can access the API directly at `http://localhost:8080`.

### Local development

**Backend:**

```bash
cp config.example.yaml config.yaml   # edit at minimum the auth.api_keys section
make run                              # builds and runs the Go server on :8080
```

**Frontend:**

```bash
cd web
npm install
npm run dev                           # Next.js dev server on http://localhost:3000
```

Both the API server and the frontend dev server need to be running for the full experience. The frontend connects to the API via the browser's origin, so in development it proxies to `localhost:8080` for API calls and WebSocket.

## Connecting Ralph Loop

Point your [Ralph Loop TUI](https://github.com/fireynis/ralph-loop-tui) instance at the hub:

```bash
ralph-loop \
  -hub-url http://ralph-hub.home.arpa \
  -hub-api-key rhk_abc123 \
  -instance-id my-app/BD-42
```

Or via environment variables:

```bash
export RALPH_HUB_URL=http://ralph-hub.home.arpa
export RALPH_HUB_API_KEY=rhk_abc123
export RALPH_INSTANCE_ID=my-app/BD-42
```

The instance ID defaults to `{repo}/{epic}` if not specified. The API key must match one of the keys configured in `config.yaml`.

## Configuration

All configuration lives in `config.yaml` (see [`config.example.yaml`](config.example.yaml) for a template). The server loads `./config.yaml` by default, or pass `-config /path/to/config.yaml`.

```yaml
server:
  port: 8080                          # HTTP listen port

storage:
  driver: sqlite                      # "sqlite" or "postgres"
  sqlite:
    path: ./ralph-hub.db              # SQLite database file path
  postgres:
    dsn: postgres://user:pass@host:5432/ralph_hub?sslmode=disable

auth:
  api_keys:                           # API keys for event ingestion (Bearer tokens)
    - name: my-agent                  # human-readable label
      key: changeme                   # token value

webhooks:                             # outbound webhook targets
  - url: https://hooks.slack.com/services/xxx
    events:                           # filter by event type (empty = all events)
      - session.ended
      - iteration.completed
    filter:
      passed_only: false              # only deliver events where passed=true

cors:
  allowed_origins:                    # CORS origins for the dashboard
    - http://localhost:3000
```

| Option | Default | Description |
|--------|---------|-------------|
| `server.port` | `8080` | HTTP listen port |
| `storage.driver` | `sqlite` | Storage backend: `sqlite` or `postgres` |
| `storage.sqlite.path` | `./ralph-hub.db` | SQLite database file path |
| `storage.postgres.dsn` | -- | Postgres connection string |
| `auth.api_keys[].name` | -- | Human-readable key label |
| `auth.api_keys[].key` | -- | Bearer token value for event ingestion |
| `webhooks[].url` | -- | Webhook delivery endpoint |
| `webhooks[].events` | `[]` (all) | Event types to deliver (empty = all) |
| `webhooks[].filter.passed_only` | `false` | Only deliver events where `passed=true` |
| `cors.allowed_origins` | `["http://localhost:3000"]` | Allowed CORS origins |

## API Reference

All routes require no authentication unless noted. The base path is `/api/v1`.

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Returns `ok`. Use for Docker health checks and load balancer probes. |

### Event Ingestion

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/events` | Bearer token | Ingest an event from a Ralph Loop instance |

**Request:** JSON body containing an event object (see [Event Schema](#event-schema) below).

**Response:** `201 Created` with the event echoed back as JSON. Returns `400` for validation errors, `401` for missing/invalid auth, `500` for storage errors.

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H "Authorization: Bearer rhk_abc123" \
  -H "Content-Type: application/json" \
  -d '{
    "event_id": "evt-001",
    "type": "iteration.completed",
    "timestamp": "2026-03-01T10:30:00Z",
    "instance_id": "my-app/BD-42",
    "repo": "my-app",
    "epic": "BD-42",
    "data": {
      "iteration": 3,
      "duration_ms": 45000,
      "passed": true,
      "task_id": "beads-123",
      "verdict": "pass"
    },
    "context": {
      "session_id": "sess-abc",
      "session_start": "2026-03-01T10:00:00Z",
      "max_iterations": 10,
      "current_iteration": 3,
      "status": "running",
      "current_phase": "dev",
      "analytics": {
        "passed_count": 2,
        "failed_count": 1,
        "tasks_closed": 1,
        "avg_duration_ms": 40000,
        "total_duration_ms": 120000
      }
    }
  }'
```

### Query Endpoints (Dashboard)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/instances` | List all instances with latest context snapshot |
| `GET` | `/api/v1/instances/{id}/history` | Iteration history for an instance. Query: `?limit=50` |
| `GET` | `/api/v1/sessions` | List sessions, paginated. Query: `?limit=50&offset=0` |
| `GET` | `/api/v1/sessions/{id}` | Session detail with full event timeline. Returns `404` if not found. |
| `GET` | `/api/v1/stats` | Aggregate statistics across all projects |

### WebSocket

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/ws` | Upgrade to WebSocket for live event streaming |

Connect via `ws://host/api/v1/ws` (or `wss://` for TLS). The server pushes every ingested event as a JSON text frame. The client read loop is used only for disconnect detection -- sent messages are ignored.

Non-blocking delivery: if a client's send buffer is full (256 messages), that message is dropped for that client rather than blocking other clients.

## Event Schema

Every event emitted by Ralph Loop follows this structure:

```json
{
  "event_id": "string",
  "type": "session.started",
  "timestamp": "2026-03-01T10:00:00Z",
  "instance_id": "my-app/BD-42",
  "repo": "my-app",
  "epic": "BD-42",
  "data": { },
  "context": {
    "session_id": "string",
    "session_start": "2026-03-01T10:00:00Z",
    "max_iterations": 10,
    "current_iteration": 1,
    "status": "running",
    "current_phase": "dev",
    "analytics": {
      "passed_count": 0,
      "failed_count": 0,
      "tasks_closed": 0,
      "initial_ready": 5,
      "current_ready": 4,
      "avg_duration_ms": 0,
      "total_duration_ms": 0
    }
  }
}
```

### Event Types

| Type | When Emitted | Key `data` Fields |
|------|-------------|-------------------|
| `session.started` | Ralph Loop begins a run | `max_iterations` |
| `session.ended` | Loop finishes or is interrupted | `reason` (complete / interrupted / error) |
| `iteration.started` | New iteration begins | `iteration`, `phase` |
| `iteration.completed` | Iteration finishes | `duration_ms`, `task_id`, `passed`, `notes`, `review_cycles`, `verdict` |
| `phase.changed` | Pipeline phase transition | `from_phase`, `to_phase` |
| `task.claimed` | Ralph picks up a task from the tracker | `task_id`, `priority`, `description` |
| `task.closed` | Task completed and committed | `task_id`, `commit_hash` |

### Context Envelope

Every event includes a `context` object with the full session state snapshot at emission time. This means the dashboard can reconstruct the complete state of any instance from a single event -- no need to replay history.

**Required fields:** `event_id`, `type`, `timestamp`, `instance_id`, `repo`, `context` (with `session_id`, `session_start`, `max_iterations`, `current_iteration`, `status`, `current_phase`).

## Dashboard

The Next.js frontend provides a real-time monitoring dashboard with the following pages:

| Page | Path | Description |
|------|------|-------------|
| **Overview** | `/` | Grid of active and inactive instance cards with live status updates |
| **Instance Detail** | `/instances/[id]` | Phase indicator, pass rate and duration charts (Recharts), iteration history table |
| **Sessions** | `/sessions` | Paginated session list with search |
| **Session Detail** | `/sessions/[id]` | Full event timeline for a session with stats summary |
| **Settings** | `/settings` | Webhook configuration display |

The dashboard connects via WebSocket on page load and receives every event in real time. Initial state is fetched from the REST API. State is managed with Zustand, and the UI updates instantly as events arrive.

### Tech Stack

- **Next.js 16** with App Router
- **React 19**
- **Tailwind CSS 4** (dark theme)
- **Zustand 5** for client state
- **Recharts 3** for data visualization

## Webhook Dispatcher

Outbound webhooks are delivered asynchronously and never block event ingestion.

- **Filtering:** Webhooks can be scoped to specific event types and optionally restricted to passing iterations only.
- **Delivery:** HTTP POST with JSON body, `Content-Type: application/json`.
- **Retries:** 3 attempts with exponential backoff (1s, 2s, 4s). Stops on first 2xx response.
- **Concurrency:** Each delivery runs in its own goroutine.

## Storage

Ralph Hub supports two storage backends:

### SQLite (default)

Uses [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) -- a pure-Go SQLite implementation with no CGO dependency. The database is created automatically on first run.

- WAL journal mode for concurrent reads
- Busy timeout of 5 seconds

### PostgreSQL

Uses [lib/pq](https://github.com/lib/pq). Provide the DSN in `storage.postgres.dsn`. Tables are created automatically via `CREATE TABLE IF NOT EXISTS`.

Both implementations share the same `Store` interface:

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

## Development

### Build targets

```bash
make build          # go build -o ralph-hub ./cmd/hub
make run            # build + run
make test           # go test ./...
make lint           # golangci-lint run
make clean          # remove binary
```

### Running tests

```bash
# Go tests (uses in-memory SQLite, no external dependencies)
go test ./...

# Postgres tests (optional, requires a running Postgres instance)
RALPH_HUB_POSTGRES_DSN="postgres://..." go test ./internal/store/...

# Frontend type-check + build
cd web && npm run build

# Frontend lint
cd web && npm run lint
```

### Project structure

```
ralph-hub/
├── cmd/hub/main.go              # Entry point: config, store, hub, dispatcher, server wiring
├── internal/
│   ├── config/                  # YAML config loading with defaults
│   ├── events/                  # Event types, struct definitions, validation
│   ├── server/                  # HTTP server, routes (Go 1.22+ patterns), handlers, middleware
│   ├── store/                   # Store interface + SQLite and Postgres implementations
│   ├── webhook/                 # Outbound webhook dispatcher with retries
│   └── ws/                      # WebSocket hub (fan-out broadcast)
├── web/                         # Next.js 16 dashboard
│   └── src/
│       ├── app/                 # App Router pages
│       ├── components/          # UI components (instance cards, phase indicator, charts)
│       ├── hooks/               # useWebSocket (auto-reconnect, backoff)
│       ├── lib/                 # API client utilities, types
│       └── store/               # Zustand stores (instances, events)
├── config.example.yaml          # Example configuration
├── Dockerfile                   # Multi-stage Go build (golang:1.25-alpine → alpine:3.21)
├── web/Dockerfile               # Multi-stage Node build (node:22-alpine)
├── docker-compose.yml           # Two services with Traefik labels
└── Makefile                     # Build, test, lint, run targets
```

## Docker Deployment

The `docker-compose.yml` runs two services behind a Traefik reverse proxy:

| Service | Container | Port | Traefik Route |
|---------|-----------|------|---------------|
| `ralph-hub` | Go API server | 8080 | `ralph-hub.home.arpa` + `/api/*` or `/healthz` (priority 20) |
| `ralph-hub-web` | Next.js dashboard | 3000 | `ralph-hub.home.arpa` (priority 10, catch-all) |

The API routes take priority so that `/api/*` and `/healthz` hit the Go server, while all other paths fall through to the Next.js frontend.

**Prerequisites:**
- External Docker network named `proxy` (`docker network create proxy`)
- Traefik running on the same network
- DNS resolving `ralph-hub.home.arpa` to the Docker host

## License

This project is not currently licensed for external use.

# Single Binary Release Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Embed the Next.js frontend into the Go binary and automate cross-platform releases via GitHub Actions.

**Architecture:** Next.js static export produces plain HTML/CSS/JS in `web/out/`. A build script copies this to `internal/frontend/dist/`, which Go embeds via `embed.FS`. The server serves embedded files at `/` with SPA fallback for client-side routing. GitHub Actions builds and releases on `v*` tag pushes.

**Tech Stack:** Go 1.25 (embed.FS), Next.js 16 (static export), GitHub Actions, `gh release create`

---

### Task 1: Configure Next.js Static Export

**Files:**
- Modify: `web/next.config.ts`
- Modify: `web/src/app/instances/[id]/page.tsx`
- Modify: `web/src/app/sessions/[id]/page.tsx`

Next.js static export (`output: 'export'`) requires dynamic route pages to have `generateStaticParams`. Since these pages are entirely client-rendered (data fetched via `useParams` + `fetch`), we return an empty array — no pages are pre-rendered, and the Go server's SPA fallback handles direct URL access.

Also, `next/font/google` (Geist fonts in `layout.tsx`) does NOT work with static export. It must be replaced with a `<link>` tag or local font files.

**Step 1: Change next.config.ts from standalone to export**

Replace `web/next.config.ts`:

```ts
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
};

export default nextConfig;
```

**Step 2: Add generateStaticParams to dynamic route pages**

In `web/src/app/instances/[id]/page.tsx`, add after the imports (before the component):

```tsx
export function generateStaticParams() {
  return [];
}
```

In `web/src/app/sessions/[id]/page.tsx`, add the same:

```tsx
export function generateStaticParams() {
  return [];
}
```

**Step 3: Fix Google Fonts for static export**

The layout uses `next/font/google` which doesn't work with `output: 'export'`. Replace the font setup in `web/src/app/layout.tsx`:

Remove the Geist/Geist_Mono imports and font instantiation. Replace with a simple `<link>` in the `<head>`:

```tsx
import type { Metadata } from 'next';
import './globals.css';
import { LayoutShell } from '@/components/layout-shell';

export const metadata: Metadata = {
  title: 'Ralph Hub',
  description: 'Centralized Ralph Loop monitoring dashboard',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <head>
        <link
          href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="font-[Geist] antialiased">
        <LayoutShell>{children}</LayoutShell>
      </body>
    </html>
  );
}
```

Update any Tailwind classes referencing the CSS variables (`--font-geist-sans`, `--font-geist-mono`) — check if they exist in `globals.css` or components.

**Step 4: Verify the static export builds**

Run: `cd web && npm run build`
Expected: build succeeds, `web/out/` directory is created with HTML/CSS/JS files.

**Step 5: Commit**

```bash
git add web/next.config.ts web/src/app/instances/\[id\]/page.tsx web/src/app/sessions/\[id\]/page.tsx web/src/app/layout.tsx
git commit -m "feat: configure Next.js static export for embedding"
```

---

### Task 2: Create the Go Embed Package

**Files:**
- Create: `internal/frontend/embed.go`
- Create: `internal/frontend/dist/.gitkeep`
- Modify: `.gitignore`

**Step 1: Create the dist directory with placeholder**

```bash
mkdir -p internal/frontend/dist
touch internal/frontend/dist/.gitkeep
```

**Step 2: Create embed.go**

Create `internal/frontend/embed.go`:

```go
package frontend

import "embed"

// Dist contains the static frontend build output.
// Populated by copying web/out/ to internal/frontend/dist/ before building.
//
//go:embed all:dist
var Dist embed.FS
```

**Step 3: Add dist contents to .gitignore**

Append to `.gitignore`:

```
internal/frontend/dist/
!internal/frontend/dist/.gitkeep
```

**Step 4: Verify it compiles**

Run: `go build ./internal/frontend/`
Expected: compiles (embeds the `.gitkeep` file).

**Step 5: Commit**

```bash
git add internal/frontend/embed.go internal/frontend/dist/.gitkeep .gitignore
git commit -m "feat: add frontend embed package with dist placeholder"
```

---

### Task 3: Serve Embedded Frontend from the Go Server

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server.go` (Handler method)

The server needs to serve the embedded filesystem for non-API routes, with SPA fallback (serve `index.html` for routes that don't match a static file).

**Step 1: Add frontend FS field to Server and New()**

Modify `internal/server/server.go`. Add `"io/fs"` to imports. Add a field to the Server struct:

```go
type Server struct {
	store      store.Store
	hub        *ws.Hub
	dispatcher *webhook.Dispatcher
	config     config.Config
	upgrader   websocket.Upgrader
	frontendFS fs.FS // embedded frontend files, may be nil
}
```

Add an optional setter or modify `New()` to accept it. The cleanest approach: add a method:

```go
// SetFrontendFS sets the embedded frontend filesystem for serving static files.
func (s *Server) SetFrontendFS(fsys fs.FS) {
	s.frontendFS = fsys
}
```

**Step 2: Add SPA file handler to Handler()**

At the end of the `Handler()` method, before `return corsMiddleware(...)`, add the frontend handler. If `s.frontendFS` is set, mount it as a catch-all:

```go
if s.frontendFS != nil {
	mux.Handle("/", s.spaHandler())
}
```

Add the `spaHandler` method to `server.go`:

```go
// spaHandler serves the embedded frontend files with SPA fallback.
// Known static files are served directly; everything else gets index.html.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServerFS(s.frontendFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly.
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			path = strings.TrimPrefix(path, "/")
		}

		// Check if the file exists in the embedded FS.
		f, err := s.frontendFS.Open(path)
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for client-side routing.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
```

Add `"strings"` to imports (if not already present).

**Step 3: Wire it up in main.go**

Modify `cmd/hub/main.go`. Add imports for `"io/fs"` and `"github.com/fireynis/ralph-hub/internal/frontend"`.

After creating the server (`srv := server.New(...)`), add:

```go
// Serve embedded frontend if available.
frontendFS, err := fs.Sub(frontend.Dist, "dist")
if err != nil {
	log.Fatalf("failed to access embedded frontend: %v", err)
}
srv.SetFrontendFS(frontendFS)
```

**Step 4: Verify it compiles**

Run: `go build ./cmd/hub/`
Expected: compiles. The embedded FS only contains `.gitkeep` but that's fine.

**Step 5: Commit**

```bash
git add internal/server/server.go cmd/hub/main.go
git commit -m "feat: serve embedded frontend with SPA fallback"
```

---

### Task 4: Make Config File Optional

**Files:**
- Modify: `internal/config/config.go:73-89`
- Modify: `internal/config/config_test.go`

Currently `config.Load()` fails if the file doesn't exist. We want `./ralph-hub` to work with zero config.

**Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("driver = %s, want sqlite", cfg.Storage.Driver)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoad_MissingFile -v`
Expected: FAIL (currently returns file-not-found error).

**Step 3: Fix config.Load to handle missing files gracefully**

Modify the `Load` function in `internal/config/config.go`. Add `"errors"` to imports:

```go
func Load(path string) (Config, error) {
	cfg := defaults()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: all pass.

**Step 5: Run full test suite**

Run: `go test ./... -count=1`
Expected: all pass.

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: return defaults when config file doesn't exist"
```

---

### Task 5: Update Makefile with dist target

**Files:**
- Modify: `Makefile`

**Step 1: Add dist and build-frontend targets**

Replace the Makefile with:

```makefile
.PHONY: build lint clean run test build-frontend dist

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

build: build-frontend
	go build -o ralph-hub ./cmd/hub

build-frontend:
	cd web && npm ci && npm run build
	rm -rf internal/frontend/dist
	cp -r web/out internal/frontend/dist

lint:
	golangci-lint run

clean:
	rm -f ralph-hub
	rm -rf dist/
	rm -rf internal/frontend/dist
	mkdir -p internal/frontend/dist
	touch internal/frontend/dist/.gitkeep

run: build
	./ralph-hub

test:
	go test ./...

dist: build-frontend
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -o "dist/ralph-hub-$$os-$$arch$$ext" ./cmd/hub; \
	done
	@echo "Done. Binaries in dist/"
```

**Step 2: Verify the dist target works for the current platform**

Run: `make build`
Expected: builds frontend, copies to `internal/frontend/dist/`, compiles Go binary.

**Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add dist target for cross-compiled releases"
```

---

### Task 6: Add GitHub Actions Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Step 1: Create the workflow file**

```bash
mkdir -p .github/workflows
```

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: '22'
          cache: 'npm'
          cache-dependency-path: web/package-lock.json

      - name: Build frontend
        run: |
          cd web
          npm ci
          npm run build

      - name: Copy frontend to embed directory
        run: |
          rm -rf internal/frontend/dist
          cp -r web/out internal/frontend/dist

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Run tests
        run: go test ./...

      - name: Build binaries
        run: |
          mkdir -p dist
          platforms=(
            "linux/amd64"
            "linux/arm64"
            "darwin/amd64"
            "darwin/arm64"
            "windows/amd64"
          )
          for platform in "${platforms[@]}"; do
            os="${platform%/*}"
            arch="${platform#*/}"
            ext=""
            if [ "$os" = "windows" ]; then ext=".exe"; fi
            echo "Building $os/$arch..."
            GOOS=$os GOARCH=$arch go build -o "dist/ralph-hub-${os}-${arch}${ext}" ./cmd/hub
          done

      - name: Create release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release create "${{ github.ref_name }}" \
            --title "${{ github.ref_name }}" \
            --generate-notes \
            dist/*
```

**Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add GitHub Actions release workflow for cross-platform binaries"
```

---

### Task 7: Final Verification

**Step 1: Run full Go test suite**

Run: `go test ./... -count=1`
Expected: all pass.

**Step 2: Build the full binary locally**

Run: `make build`
Expected: frontend builds, Go compiles with embedded frontend.

**Step 3: Smoke test the binary**

Run: `./ralph-hub` (no config file needed)
Expected: starts on port 8080, serves both API and frontend.

Verify: `curl -s http://localhost:8080/healthz` returns `ok`.
Verify: `curl -s http://localhost:8080/ | head -5` returns HTML.

Stop the server with Ctrl-C.

**Step 4: Verify cross-compilation works for one target**

Run: `GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/hub`
Expected: compiles without error (no CGO deps since we use modernc.org/sqlite).

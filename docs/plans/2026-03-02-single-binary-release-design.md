# Single Binary Distribution with GitHub Releases

## Problem

Ralph Hub requires running a Go API server and a Next.js frontend separately. There's no easy way for someone to download and run it locally.

## Approach

Embed the Next.js frontend (static export) into the Go binary using `embed.FS`. A GitHub Actions workflow triggered by `v*` tags builds the frontend, cross-compiles for 5 OS/arch targets, and creates a GitHub release with all binaries.

## Components

### 1. Next.js Static Export

Configure `next.config.ts` with `output: 'export'`. All pages are already `'use client'` so no behavior change. Build produces `web/out/` with plain HTML/CSS/JS.

### 2. Go Embed Package (`internal/frontend/`)

- `embed.go` with `//go:embed all:dist` directive
- Exports the embedded filesystem for the server to use
- `dist/` directory contains the frontend build output (gitignored, populated at build time)
- A placeholder `.gitkeep` file ensures the directory exists for local dev

### 3. Server Changes

- Serve embedded frontend files at `/` with SPA fallback (unknown routes → `index.html`)
- API routes (`/api/v1/*`, `/healthz`, `/api/v1/ws`) take priority over static files
- In `cmd/hub/main.go`: if config file doesn't exist, silently use defaults instead of erroring

### 4. GitHub Actions Workflow (`.github/workflows/release.yml`)

- Trigger: `push tags: ['v*']`
- Build frontend: Node 22, `npm ci && npm run build` in `web/`
- Copy `web/out/` to `internal/frontend/dist/`
- Cross-compile 5 targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- Create GitHub release with binaries using `gh release create`
- Binary naming: `ralph-hub-{os}-{arch}` (`.exe` suffix for Windows)

### 5. Config Fallback

`config.Load()` should return defaults silently when the config file doesn't exist, so `./ralph-hub` works with zero config.

### 6. Makefile Updates

Add `make dist` for local cross-compilation (same steps as CI, for testing).

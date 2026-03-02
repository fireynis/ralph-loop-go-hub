FROM node:22-alpine AS frontend

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/out internal/frontend/dist
RUN CGO_ENABLED=0 go build -o /ralph-hub ./cmd/hub

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /ralph-hub /usr/local/bin/ralph-hub
ENTRYPOINT ["ralph-hub", "-config", "/etc/ralph-hub/config.yaml"]

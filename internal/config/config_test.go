package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load('') returned error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("expected default driver 'sqlite', got %q", cfg.Storage.Driver)
	}
	if cfg.Storage.SQLite.Path != "./ralph-hub.db" {
		t.Errorf("expected default sqlite path './ralph-hub.db', got %q", cfg.Storage.SQLite.Path)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	configYAML := `
server:
  port: 9090
storage:
  driver: postgres
  postgres:
    dsn: "postgres://user:pass@localhost:5432/hub"
auth:
  api_keys:
    - name: test-key
      key: secret123
webhooks:
  - url: "http://example.com/hook"
    events:
      - heartbeat
      - session.end
    filter:
      passed_only: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Driver != "postgres" {
		t.Errorf("expected driver 'postgres', got %q", cfg.Storage.Driver)
	}
	if cfg.Storage.Postgres.DSN != "postgres://user:pass@localhost:5432/hub" {
		t.Errorf("unexpected postgres DSN: %q", cfg.Storage.Postgres.DSN)
	}
	if len(cfg.Auth.APIKeys) != 1 {
		t.Fatalf("expected 1 API key, got %d", len(cfg.Auth.APIKeys))
	}
	if cfg.Auth.APIKeys[0].Name != "test-key" {
		t.Errorf("expected API key name 'test-key', got %q", cfg.Auth.APIKeys[0].Name)
	}
	if cfg.Auth.APIKeys[0].Key != "secret123" {
		t.Errorf("expected API key 'secret123', got %q", cfg.Auth.APIKeys[0].Key)
	}
	if len(cfg.Webhooks) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(cfg.Webhooks))
	}
	if cfg.Webhooks[0].URL != "http://example.com/hook" {
		t.Errorf("unexpected webhook URL: %q", cfg.Webhooks[0].URL)
	}
	if len(cfg.Webhooks[0].Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(cfg.Webhooks[0].Events))
	}
	if !cfg.Webhooks[0].Filter.PassedOnly {
		t.Error("expected passed_only to be true")
	}
}

func TestLoadConfig_PartialYAML(t *testing.T) {
	configYAML := `
server:
  port: 9090
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("expected default driver 'sqlite' preserved, got %q", cfg.Storage.Driver)
	}
	if cfg.Storage.SQLite.Path != "./ralph-hub.db" {
		t.Errorf("expected default sqlite path './ralph-hub.db' preserved, got %q", cfg.Storage.SQLite.Path)
	}
}

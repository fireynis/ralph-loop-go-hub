package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Storage  StorageConfig   `yaml:"storage"`
	Auth     AuthConfig      `yaml:"auth"`
	Webhooks []WebhookConfig `yaml:"webhooks"`
	CORS     CORSConfig      `yaml:"cors"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type StorageConfig struct {
	Driver   string         `yaml:"driver"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Postgres PostgresConfig `yaml:"postgres"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type AuthConfig struct {
	APIKeys []APIKey `yaml:"api_keys"`
}

type APIKey struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type WebhookConfig struct {
	URL    string   `yaml:"url"`
	Events []string `yaml:"events"`
	Filter struct {
		PassedOnly bool `yaml:"passed_only"`
	} `yaml:"filter"`
}

func defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Storage: StorageConfig{
			Driver: "sqlite",
			SQLite: SQLiteConfig{
				Path: "./ralph-hub.db",
			},
		},
		CORS: CORSConfig{
			AllowedOrigins: []string{"http://localhost:3000"},
		},
	}
}

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

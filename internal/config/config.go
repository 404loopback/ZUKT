package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	ServerName    string
	ServerVersion string
	ZoektBackend  string
	ZoektHTTPURL  string
	ZoektTimeout  time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ServerName:    envOrDefault("MCP_SERVER_NAME", "zoekt-mcp-wrapper"),
		ServerVersion: envOrDefault("MCP_SERVER_VERSION", "0.1.0"),
		ZoektBackend:  envOrDefault("ZOEKT_BACKEND", "mock"),
		ZoektHTTPURL:  os.Getenv("ZOEKT_HTTP_BASE_URL"),
	}

	timeoutRaw := envOrDefault("ZOEKT_HTTP_TIMEOUT", "5s")
	timeout, err := time.ParseDuration(timeoutRaw)
	if err != nil {
		return Config{}, fmt.Errorf("invalid ZOEKT_HTTP_TIMEOUT: %w", err)
	}
	cfg.ZoektTimeout = timeout

	if cfg.ServerName == "" {
		return Config{}, fmt.Errorf("MCP_SERVER_NAME cannot be empty")
	}

	if cfg.ServerVersion == "" {
		return Config{}, fmt.Errorf("MCP_SERVER_VERSION cannot be empty")
	}

	switch cfg.ZoektBackend {
	case "mock":
	case "http":
		if cfg.ZoektHTTPURL == "" {
			return Config{}, fmt.Errorf("ZOEKT_HTTP_BASE_URL is required when ZOEKT_BACKEND=http")
		}
	default:
		return Config{}, fmt.Errorf("unsupported ZOEKT_BACKEND %q (expected mock or http)", cfg.ZoektBackend)
	}

	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/404loopback/zukt/internal/paths"
)

type Config struct {
	ServerName       string
	ServerVersion    string
	ZoektHTTPURL     string
	ZoektTimeout     time.Duration
	ZoektAllowedDirs []string
	ZoektExcludeDirs []string
	Warnings         []string
	ProjectRoot      string
}

func Load() (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolve current working directory: %w", err)
	}

	defaultAllowed := os.Getenv("HOME")
	if strings.TrimSpace(defaultAllowed) == "" {
		defaultAllowed = cwd
	}

	httpURL := envOrDefault("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")
	backend := envOrDefault("ZOEKT_BACKEND", "http")
	cfg := Config{
		ServerName:       envOrDefault("MCP_SERVER_NAME", "zukt"),
		ServerVersion:    envOrDefault("MCP_SERVER_VERSION", "0.1.0"),
		ZoektHTTPURL:     httpURL,
		ZoektAllowedDirs: paths.NormalizeAndSortUnique(parseCSV(envOrDefault("ZOEKT_ALLOWED_ROOTS", defaultAllowed))),
		ZoektExcludeDirs: parseCSV(envOrDefault("ZOEKT_EXCLUDE_DIRS", ".git,node_modules,.venv,dist,build")),
		Warnings:         deprecatedEnvWarnings(),
		ProjectRoot:      cwd,
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

	// Runtime contract: zukt is MCP/search only and always targets Zoekt HTTP.
	if backend != "http" {
		return Config{}, fmt.Errorf("unsupported ZOEKT_BACKEND=%q (runtime contract requires http)", backend)
	}
	if err := validateLocalHTTPURL(cfg.ZoektHTTPURL); err != nil {
		return Config{}, fmt.Errorf("invalid ZOEKT_HTTP_BASE_URL: %w", err)
	}

	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func deprecatedEnvWarnings() []string {
	legacyVars := []string{
		"ZOEKT_AUTOPILOT",
		"ZOEKT_REPOS",
		"ZOEKT_INDEX_DIR",
		"ZOEKT_FORCE_REINDEX",
	}

	warnings := make([]string, 0, len(legacyVars))
	for _, key := range legacyVars {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s is deprecated and ignored; it will be removed in the next release", key))
	}
	slices.Sort(warnings)
	return warnings
}

func validateLocalHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}

	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host must be localhost or loopback IP")
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("host must be localhost or loopback IP")
	}
	return nil
}

func ValidateRepoPath(repo string, allowedRoots []string) error {
	_, err := paths.ValidateRepoPath(repo, allowedRoots)
	return err
}

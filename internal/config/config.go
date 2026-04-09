package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	ServerName        string
	ServerVersion     string
	ZoektBackend      string
	ZoektHTTPURL      string
	ZoektTimeout      time.Duration
	ZoektAutopilot    bool
	ZoektRepos        []string
	ZoektIndexDir     string
	ZoektAllowedDirs  []string
	ZoektExcludeDirs  []string
	ZoektForceReindex bool
	ProjectRoot       string
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
	indexDir, err := filepath.Abs(envOrDefault("ZOEKT_INDEX_DIR", "./zoekt-index"))
	if err != nil {
		return Config{}, fmt.Errorf("resolve ZOEKT_INDEX_DIR: %w", err)
	}
	cfg := Config{
		ServerName:        envOrDefault("MCP_SERVER_NAME", "zukt"),
		ServerVersion:     envOrDefault("MCP_SERVER_VERSION", "0.1.0"),
		ZoektBackend:      envOrDefault("ZOEKT_BACKEND", "http"),
		ZoektHTTPURL:      httpURL,
		ZoektAutopilot:    envBoolOrDefault("ZOEKT_AUTOPILOT", true),
		ZoektRepos:        parseCSV(os.Getenv("ZOEKT_REPOS")),
		ZoektIndexDir:     indexDir,
		ZoektAllowedDirs:  parseCSV(envOrDefault("ZOEKT_ALLOWED_ROOTS", defaultAllowed)),
		ZoektExcludeDirs:  parseCSV(envOrDefault("ZOEKT_EXCLUDE_DIRS", ".git,node_modules,.venv,dist,build")),
		ZoektForceReindex: envBoolOrDefault("ZOEKT_FORCE_REINDEX", false),
		ProjectRoot:       cwd,
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
	case "http":
		if err := validateLocalHTTPURL(cfg.ZoektHTTPURL); err != nil {
			return Config{}, fmt.Errorf("invalid ZOEKT_HTTP_BASE_URL: %w", err)
		}
	case "mock":
	default:
		return Config{}, fmt.Errorf("unsupported ZOEKT_BACKEND %q (expected mock or http)", cfg.ZoektBackend)
	}

	if cfg.ZoektAutopilot {
		if cfg.ZoektBackend != "http" {
			return Config{}, fmt.Errorf("ZOEKT_AUTOPILOT requires ZOEKT_BACKEND=http")
		}
		for _, repo := range cfg.ZoektRepos {
			if err := ValidateRepoPath(repo, cfg.ZoektAllowedDirs); err != nil {
				return Config{}, err
			}
		}
	}

	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBoolOrDefault(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
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
	if !filepath.IsAbs(repo) {
		return fmt.Errorf("repo path must be absolute: %s", repo)
	}
	info, err := os.Stat(repo)
	if err != nil {
		return fmt.Errorf("repo path not accessible %s: %w", repo, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repo path is not a directory: %s", repo)
	}
	if len(allowedRoots) == 0 {
		return fmt.Errorf("ZOEKT_ALLOWED_ROOTS cannot be empty")
	}

	repoEval, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return fmt.Errorf("resolve repo symlinks %s: %w", repo, err)
	}

	for _, root := range allowedRoots {
		rootEval := root
		if !filepath.IsAbs(rootEval) {
			absRoot, absErr := filepath.Abs(rootEval)
			if absErr == nil {
				rootEval = absRoot
			}
		}
		if resolved, err := filepath.EvalSymlinks(rootEval); err == nil {
			rootEval = resolved
		}
		rel, err := filepath.Rel(rootEval, repoEval)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && rel != "") {
			return nil
		}
	}
	return fmt.Errorf("repo path %s is outside ZOEKT_ALLOWED_ROOTS", repo)
}

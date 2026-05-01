package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
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
	Semantic         SemanticConfig
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
	semanticCfg, err := loadSemanticConfig()
	if err != nil {
		return Config{}, err
	}
	cfg.Semantic = semanticCfg

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

func parseIntEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}

func parseInt64Env(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
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

func ValidateRepoPath(repo string, allowedRoots []string) error {
	_, err := paths.ValidateRepoPath(repo, allowedRoots)
	return err
}

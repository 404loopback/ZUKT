package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAutopilotAllowsEmptyEnvRepos(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_AUTOPILOT", "true")
	t.Setenv("ZOEKT_REPOS", "")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")

	_, err := Load()
	if err != nil {
		t.Fatalf("expected no error with empty env repos (manager file may provide repos), got: %v", err)
	}
}

func TestLoadRejectsNonLocalHTTPURL(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_AUTOPILOT", "false")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://example.com:6070")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for non-local ZOEKT_HTTP_BASE_URL")
	}
}

func TestLoadValidatesRepoAgainstAllowedRoots(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_AUTOPILOT", "true")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")
	t.Setenv("ZOEKT_REPOS", repo)
	t.Setenv("ZOEKT_ALLOWED_ROOTS", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if len(cfg.ZoektRepos) != 1 || cfg.ZoektRepos[0] != repo {
		t.Fatalf("unexpected repos: %#v", cfg.ZoektRepos)
	}
}

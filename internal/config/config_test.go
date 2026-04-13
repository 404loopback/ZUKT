package config

import "testing"

func TestLoadRejectsNonLocalHTTPURL(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://example.com:6070")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for non-local ZOEKT_HTTP_BASE_URL")
	}
}

func TestLoadRejectsUnsupportedBackend(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "mock")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for non-http backend")
	}
}

func TestLoadKeepsWorkingWithLegacyVarsButWarns(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")
	t.Setenv("ZOEKT_AUTOPILOT", "true")
	t.Setenv("ZOEKT_REPOS", "/tmp/legacy-repo")
	t.Setenv("ZOEKT_INDEX_DIR", "/tmp/legacy-index")
	t.Setenv("ZOEKT_FORCE_REINDEX", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if len(cfg.Warnings) != 4 {
		t.Fatalf("expected 4 deprecation warnings, got %d: %#v", len(cfg.Warnings), cfg.Warnings)
	}
}

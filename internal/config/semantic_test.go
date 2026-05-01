package config

import "testing"

func TestLoadSemanticConfigDefaults(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Semantic.Backend == "" {
		t.Fatalf("expected semantic backend default")
	}
	if cfg.Semantic.EmbeddingModel == "" {
		t.Fatalf("expected embedding model default")
	}
}

func TestLoadRejectsQdrantWithoutEmbeddingProvider(t *testing.T) {
	t.Setenv("ZOEKT_BACKEND", "http")
	t.Setenv("ZOEKT_HTTP_BASE_URL", "http://127.0.0.1:6070")
	t.Setenv("ZUKT_SEMANTIC_BACKEND", "qdrant")
	t.Setenv("ZUKT_EMBEDDING_PROVIDER", "disabled")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for qdrant backend without embedding provider")
	}
}

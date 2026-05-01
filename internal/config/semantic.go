package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	SemanticBackendDisabled = "disabled"
	SemanticBackendHash     = "hash"
	SemanticBackendQdrant   = "qdrant"

	EmbeddingProviderDisabled = "disabled"
	EmbeddingProviderOllama   = "ollama"
)

type SemanticConfig struct {
	Backend          string
	QdrantURL        string
	CollectionPrefix string

	EmbeddingProvider string
	OllamaURL         string
	EmbeddingModel    string
	EmbeddingDim      int

	ChunkLines   int
	ChunkOverlap int
	MaxFileBytes int64
}

func loadSemanticConfig() (SemanticConfig, error) {
	cfg := SemanticConfig{
		Backend:           strings.ToLower(strings.TrimSpace(envOrDefault("ZUKT_SEMANTIC_BACKEND", SemanticBackendHash))),
		QdrantURL:         strings.TrimSpace(envOrDefault("ZUKT_QDRANT_URL", "http://127.0.0.1:6333")),
		CollectionPrefix:  strings.TrimSpace(envOrDefault("ZUKT_QDRANT_COLLECTION_PREFIX", "zukt")),
		EmbeddingProvider: strings.ToLower(strings.TrimSpace(envOrDefault("ZUKT_EMBEDDING_PROVIDER", EmbeddingProviderOllama))),
		OllamaURL:         strings.TrimSpace(envOrDefault("ZUKT_OLLAMA_URL", "http://127.0.0.1:11434")),
		EmbeddingModel:    strings.TrimSpace(envOrDefault("ZUKT_EMBEDDING_MODEL", "nomic-embed-text")),
		EmbeddingDim:      768,
		ChunkLines:        80,
		ChunkOverlap:      20,
		MaxFileBytes:      1 << 20,
	}

	if cfg.Backend == "" {
		cfg.Backend = SemanticBackendHash
	}
	switch cfg.Backend {
	case SemanticBackendDisabled, SemanticBackendHash, SemanticBackendQdrant:
	default:
		return SemanticConfig{}, fmt.Errorf("invalid ZUKT_SEMANTIC_BACKEND=%q (expected disabled|hash|qdrant)", cfg.Backend)
	}

	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = EmbeddingProviderOllama
	}
	switch cfg.EmbeddingProvider {
	case EmbeddingProviderDisabled, EmbeddingProviderOllama:
	default:
		return SemanticConfig{}, fmt.Errorf("invalid ZUKT_EMBEDDING_PROVIDER=%q (expected disabled|ollama)", cfg.EmbeddingProvider)
	}

	if dim, err := parseIntEnv("ZUKT_EMBEDDING_DIM", cfg.EmbeddingDim); err != nil {
		return SemanticConfig{}, err
	} else {
		cfg.EmbeddingDim = dim
	}
	if cfg.EmbeddingDim <= 0 {
		return SemanticConfig{}, fmt.Errorf("ZUKT_EMBEDDING_DIM must be > 0")
	}

	if lines, err := parseIntEnv("ZUKT_CHUNK_LINES", cfg.ChunkLines); err != nil {
		return SemanticConfig{}, err
	} else {
		cfg.ChunkLines = lines
	}
	if cfg.ChunkLines <= 0 {
		return SemanticConfig{}, fmt.Errorf("ZUKT_CHUNK_LINES must be > 0")
	}

	if overlap, err := parseIntEnv("ZUKT_CHUNK_OVERLAP", cfg.ChunkOverlap); err != nil {
		return SemanticConfig{}, err
	} else {
		cfg.ChunkOverlap = overlap
	}
	if cfg.ChunkOverlap < 0 {
		return SemanticConfig{}, fmt.Errorf("ZUKT_CHUNK_OVERLAP must be >= 0")
	}
	if cfg.ChunkOverlap >= cfg.ChunkLines {
		return SemanticConfig{}, fmt.Errorf("ZUKT_CHUNK_OVERLAP must be smaller than ZUKT_CHUNK_LINES")
	}

	if maxBytes, err := parseInt64Env("ZUKT_MAX_FILE_BYTES", cfg.MaxFileBytes); err != nil {
		return SemanticConfig{}, err
	} else {
		cfg.MaxFileBytes = maxBytes
	}
	if cfg.MaxFileBytes <= 0 {
		return SemanticConfig{}, fmt.Errorf("ZUKT_MAX_FILE_BYTES must be > 0")
	}

	if cfg.Backend == SemanticBackendQdrant {
		if err := validateLocalHTTPURL(cfg.QdrantURL); err != nil {
			return SemanticConfig{}, fmt.Errorf("invalid ZUKT_QDRANT_URL: %w", err)
		}
		if cfg.CollectionPrefix == "" {
			return SemanticConfig{}, fmt.Errorf("ZUKT_QDRANT_COLLECTION_PREFIX cannot be empty")
		}
	}

	if cfg.EmbeddingProvider == EmbeddingProviderOllama {
		if err := validateLocalHTTPURL(cfg.OllamaURL); err != nil {
			return SemanticConfig{}, fmt.Errorf("invalid ZUKT_OLLAMA_URL: %w", err)
		}
		if cfg.EmbeddingModel == "" {
			return SemanticConfig{}, fmt.Errorf("ZUKT_EMBEDDING_MODEL cannot be empty")
		}
	}

	if cfg.Backend == SemanticBackendQdrant && cfg.EmbeddingProvider == EmbeddingProviderDisabled {
		return SemanticConfig{}, fmt.Errorf("ZUKT_EMBEDDING_PROVIDER cannot be disabled when ZUKT_SEMANTIC_BACKEND=qdrant")
	}

	return cfg, nil
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

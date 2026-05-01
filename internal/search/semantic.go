package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/404loopback/zukt/internal/zoekt"
)

const (
	searchModeLexical  = "lexical"
	searchModeSemantic = "semantic"
	searchModeHybrid   = "hybrid"
)

type Repo struct {
	Name string
	Root string
}

type SemanticStats struct {
	Repo           string `json:"repo"`
	Backend        string `json:"backend"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
	FilesSeen      int    `json:"files_seen"`
	FilesIndexed   int    `json:"files_indexed"`
	ChunksIndexed  int    `json:"chunks_indexed"`
	ChunksSkipped  int    `json:"chunks_skipped"`
	DurationMS     int64  `json:"duration_ms"`
}

type SemanticStatus struct {
	Backend          string `json:"backend"`
	Enabled          bool   `json:"enabled"`
	Ready            bool   `json:"ready"`
	Message          string `json:"message,omitempty"`
	EmbeddingModel   string `json:"embedding_model,omitempty"`
	EmbeddingDim     int    `json:"embedding_dim,omitempty"`
	CollectionPrefix string `json:"collection_prefix,omitempty"`
}

type SemanticSearchRequest struct {
	Repo  Repo
	Query string
	Limit int
}

type SemanticHit struct {
	Repo      string  `json:"repo"`
	File      string  `json:"file"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line,omitempty"`
	Snippet   string  `json:"snippet"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`
}

type SemanticBackend interface {
	Prepare(ctx context.Context, repo Repo) (*SemanticStats, error)
	Search(ctx context.Context, req SemanticSearchRequest) ([]SemanticHit, error)
	Status(ctx context.Context) (*SemanticStatus, error)
}

type SemanticIndexStats struct {
	Repo           string `json:"repo"`
	Backend        string `json:"backend"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
	FilesSeen      int    `json:"files_seen"`
	FilesIndexed   int    `json:"files_indexed"`
	ChunksIndexed  int    `json:"chunks_indexed"`
	ChunksSkipped  int    `json:"chunks_skipped"`
	DurationMS     int64  `json:"duration_ms"`
}

type SemanticRuntimeConfig struct {
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

type ServiceOption func(*serviceOptions)

type serviceOptions struct {
	semanticConfig SemanticRuntimeConfig
}

func defaultServiceOptions() serviceOptions {
	return serviceOptions{
		semanticConfig: SemanticRuntimeConfig{
			Backend:           "hash",
			QdrantURL:         "http://127.0.0.1:6333",
			CollectionPrefix:  "zukt",
			EmbeddingProvider: "ollama",
			OllamaURL:         "http://127.0.0.1:11434",
			EmbeddingModel:    "nomic-embed-text",
			EmbeddingDim:      768,
			ChunkLines:        80,
			ChunkOverlap:      20,
			MaxFileBytes:      1 << 20,
		},
	}
}

func WithSemanticConfig(cfg SemanticRuntimeConfig) ServiceOption {
	return func(opts *serviceOptions) {
		opts.semanticConfig = cfg
	}
}

func normalizeSearchMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return searchModeSemantic
	}
	switch mode {
	case searchModeLexical, searchModeSemantic, searchModeHybrid:
		return mode
	default:
		return mode
	}
}

func semanticStatsOrEmpty(stats *SemanticStats) SemanticIndexStats {
	if stats == nil {
		return SemanticIndexStats{}
	}
	return SemanticIndexStats{
		Repo:           stats.Repo,
		Backend:        stats.Backend,
		EmbeddingModel: stats.EmbeddingModel,
		FilesSeen:      stats.FilesSeen,
		FilesIndexed:   stats.FilesIndexed,
		ChunksIndexed:  stats.ChunksIndexed,
		ChunksSkipped:  stats.ChunksSkipped,
		DurationMS:     stats.DurationMS,
	}
}

func semanticHitsToSearchResults(hits []SemanticHit) []zoekt.SearchResult {
	results := make([]zoekt.SearchResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, zoekt.SearchResult{
			Repo:    hit.Repo,
			File:    hit.File,
			Line:    hit.StartLine,
			EndLine: hit.EndLine,
			Snippet: hit.Snippet,
			Score:   hit.Score,
			Source:  hit.Source,
		})
	}
	return results
}

func unsupportedModeError(mode string) error {
	return fmt.Errorf("mode %q is not supported (expected lexical|semantic|hybrid)", mode)
}

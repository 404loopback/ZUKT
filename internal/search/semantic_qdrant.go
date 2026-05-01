package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var collectionNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type qdrantSemanticBackend struct {
	service  *Service
	cfg      SemanticRuntimeConfig
	embedder Embedder
	client   *http.Client
}

func newQdrantSemanticBackend(service *Service, cfg SemanticRuntimeConfig) (*qdrantSemanticBackend, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider))
	if provider != "ollama" {
		return nil, fmt.Errorf("unsupported embedding provider for qdrant backend: %q", cfg.EmbeddingProvider)
	}
	embedder := newOllamaEmbedder(cfg.OllamaURL, cfg.EmbeddingModel, cfg.EmbeddingDim)
	return &qdrantSemanticBackend{
		service:  service,
		cfg:      cfg,
		embedder: embedder,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}, nil
}

func (b *qdrantSemanticBackend) Prepare(ctx context.Context, repo Repo) (*SemanticStats, error) {
	chunks, buildStats, err := b.service.buildChunks(ctx, repo)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("semantic index has no chunks for repo %q", repo.Name)
	}

	collection := b.collectionName(repo)
	if err := b.recreateCollection(ctx, collection); err != nil {
		return nil, err
	}

	batchSize := 32
	indexed := 0
	skipped := buildStats.chunksSkipped
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]

		inputs := make([]string, 0, len(batch))
		for _, chunk := range batch {
			inputs = append(inputs, chunk.Content)
		}
		vectors, err := b.embedder.EmbedBatch(ctx, inputs)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", start, end, err)
		}
		if len(vectors) != len(batch) {
			return nil, fmt.Errorf("embedding batch mismatch: expected=%d got=%d", len(batch), len(vectors))
		}

		points := make([]map[string]any, 0, len(batch))
		for i, chunk := range batch {
			if len(vectors[i]) == 0 {
				skipped++
				continue
			}
			points = append(points, map[string]any{
				"id":     chunk.ChunkHash,
				"vector": vectors[i],
				"payload": map[string]any{
					"repo":       chunk.Repo,
					"repo_path":  chunk.RepoPath,
					"file":       chunk.File,
					"start_line": chunk.StartLine,
					"end_line":   chunk.EndLine,
					"language":   chunk.Language,
					"chunk_hash": chunk.ChunkHash,
					"content":    chunk.Content,
				},
			})
		}

		if len(points) == 0 {
			continue
		}
		if err := b.upsertPoints(ctx, collection, points); err != nil {
			return nil, err
		}
		indexed += len(points)
	}

	return &SemanticStats{
		Repo:           repo.Name,
		Backend:        "qdrant",
		EmbeddingModel: b.embedder.Model(),
		FilesSeen:      buildStats.filesSeen,
		FilesIndexed:   buildStats.filesIndexed,
		ChunksIndexed:  indexed,
		ChunksSkipped:  skipped,
		DurationMS:     buildStats.durationMS,
	}, nil
}

func (b *qdrantSemanticBackend) Search(ctx context.Context, req SemanticSearchRequest) ([]SemanticHit, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if req.Limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	if strings.TrimSpace(req.Repo.Name) == "" {
		return nil, fmt.Errorf("repo is required for semantic mode")
	}

	queryVector, err := b.embedder.Embed(ctx, normalizeSemanticQuery(req.Query))
	if err != nil {
		return nil, err
	}

	collection := b.collectionName(req.Repo)
	results, err := b.searchPoints(ctx, collection, queryVector, req.Limit*3)
	if err != nil {
		return nil, err
	}

	hits := make([]SemanticHit, 0, len(results))
	for _, item := range results {
		file := payloadString(item.Payload, "file")
		if file == "" {
			continue
		}
		hits = append(hits, SemanticHit{
			Repo:      payloadString(item.Payload, "repo"),
			File:      file,
			StartLine: payloadInt(item.Payload, "start_line"),
			EndLine:   payloadInt(item.Payload, "end_line"),
			Snippet:   buildPayloadSnippet(item.Payload),
			Score:     item.Score,
			Source:    "vector",
		})
		if len(hits) >= req.Limit {
			break
		}
	}
	return hits, nil
}

func (b *qdrantSemanticBackend) Status(ctx context.Context) (*SemanticStatus, error) {
	status := &SemanticStatus{
		Backend:          "qdrant",
		Enabled:          true,
		Ready:            false,
		EmbeddingModel:   b.embedder.Model(),
		EmbeddingDim:     b.embedder.Dimension(),
		CollectionPrefix: b.cfg.CollectionPrefix,
	}

	if err := b.qdrantHealthcheck(ctx); err != nil {
		status.Message = fmt.Sprintf("qdrant unavailable: %v", err)
		return status, nil
	}
	status.Ready = true
	status.Message = "qdrant + ollama embedding backend available"
	return status, nil
}

func (b *qdrantSemanticBackend) collectionName(repo Repo) string {
	name := strings.ToLower(strings.TrimSpace(repo.Name))
	if name == "" {
		name = strings.TrimSpace(repo.Root)
	}
	name = collectionNameSanitizer.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "repo"
	}
	return b.cfg.CollectionPrefix + "-" + name
}

func (b *qdrantSemanticBackend) recreateCollection(ctx context.Context, collection string) error {
	_ = b.requestJSON(ctx, http.MethodDelete, "/collections/"+url.PathEscape(collection), nil, nil)

	payload := map[string]any{
		"vectors": map[string]any{
			"size":     b.embedder.Dimension(),
			"distance": "Cosine",
		},
	}
	return b.requestJSON(ctx, http.MethodPut, "/collections/"+url.PathEscape(collection), payload, nil)
}

func (b *qdrantSemanticBackend) upsertPoints(ctx context.Context, collection string, points []map[string]any) error {
	payload := map[string]any{"points": points}
	return b.requestJSON(ctx, http.MethodPut, "/collections/"+url.PathEscape(collection)+"/points?wait=true", payload, nil)
}

type qdrantSearchPoint struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

func (b *qdrantSemanticBackend) searchPoints(ctx context.Context, collection string, vector []float32, limit int) ([]qdrantSearchPoint, error) {
	if limit <= 0 {
		limit = 10
	}
	payload := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	var response struct {
		Result []qdrantSearchPoint `json:"result"`
	}
	if err := b.requestJSON(ctx, http.MethodPost, "/collections/"+url.PathEscape(collection)+"/points/search", payload, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (b *qdrantSemanticBackend) qdrantHealthcheck(ctx context.Context) error {
	var response any
	if err := b.requestJSON(ctx, http.MethodGet, "/collections", nil, &response); err != nil {
		return err
	}
	return nil
}

func (b *qdrantSemanticBackend) requestJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	var bodyReader io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}

	base := strings.TrimRight(strings.TrimSpace(b.cfg.QdrantURL), "/")
	req, err := http.NewRequestWithContext(ctx, method, base+path, bodyReader)
	if err != nil {
		return err
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		trimmed := strings.TrimSpace(string(body))
		if trimmed == "" {
			trimmed = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("qdrant %s %s failed: status=%d body=%s", method, path, resp.StatusCode, trimmed)
	}
	if responseBody == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, responseBody); err != nil {
		return fmt.Errorf("parse qdrant response: %w", err)
	}
	return nil
}

func payloadString(payload map[string]any, key string) string {
	value, _ := payload[key]
	stringValue, _ := value.(string)
	return strings.TrimSpace(stringValue)
}

func payloadInt(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	default:
		return 0
	}
}

func buildPayloadSnippet(payload map[string]any) string {
	content := payloadString(payload, "content")
	if content == "" {
		return ""
	}
	lines := splitNormalizedLines(content)
	return semanticSnippet(lines)
}

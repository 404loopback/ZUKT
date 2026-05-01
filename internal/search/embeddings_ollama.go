package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ollamaEmbedder struct {
	baseURL string
	model   string
	dim     int
	client  *http.Client
}

func newOllamaEmbedder(baseURL, model string, dim int) *ollamaEmbedder {
	return &ollamaEmbedder{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		model:   strings.TrimSpace(model),
		dim:     dim,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (e *ollamaEmbedder) Embed(ctx context.Context, input string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("ollama returned no embedding")
	}
	return vectors[0], nil
}

func (e *ollamaEmbedder) EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return [][]float32{}, nil
	}

	requestBody := map[string]any{
		"model": e.model,
		"input": inputs,
	}
	if e.dim > 0 {
		requestBody["dimensions"] = e.dim
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama embed request: %w", err)
	}

	url := e.baseURL + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embed request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse ollama embed response: %w", err)
	}
	if len(parsed.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed response has no embeddings")
	}

	out := make([][]float32, 0, len(parsed.Embeddings))
	for _, vector64 := range parsed.Embeddings {
		vector32 := make([]float32, 0, len(vector64))
		for _, v := range vector64 {
			vector32 = append(vector32, float32(v))
		}
		if e.dim > 0 && len(vector32) != e.dim {
			return nil, fmt.Errorf("embedding dimension mismatch: expected=%d got=%d", e.dim, len(vector32))
		}
		out = append(out, vector32)
	}

	if len(out) != len(inputs) {
		return nil, fmt.Errorf("ollama returned %d embeddings for %d inputs", len(out), len(inputs))
	}
	return out, nil
}

func (e *ollamaEmbedder) Dimension() int {
	return e.dim
}

func (e *ollamaEmbedder) Model() string {
	return e.model
}

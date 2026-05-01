package search

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type hashSemanticBackend struct {
	service *Service
	mu      sync.RWMutex
	indexes map[string]*hashSemanticIndex
}

type hashSemanticIndex struct {
	repo       Repo
	chunks     []semanticChunk
	fileChunks map[string][]int
	stats      SemanticStats
}

type scoredSemanticChunk struct {
	chunk semanticChunk
	score float64
}

func newHashSemanticBackend(service *Service) *hashSemanticBackend {
	return &hashSemanticBackend{
		service: service,
		indexes: make(map[string]*hashSemanticIndex),
	}
}

func (b *hashSemanticBackend) Prepare(ctx context.Context, repo Repo) (*SemanticStats, error) {
	index, err := b.buildIndex(ctx, repo)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.indexes[repo.Root] = index
	b.mu.Unlock()
	copyStats := index.stats
	return &copyStats, nil
}

func (b *hashSemanticBackend) Search(ctx context.Context, req SemanticSearchRequest) ([]SemanticHit, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if req.Limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	if strings.TrimSpace(req.Repo.Root) == "" {
		return nil, fmt.Errorf("repo root is required")
	}

	index, err := b.ensureIndex(ctx, req.Repo)
	if err != nil {
		return nil, err
	}

	queryVector := semanticVector(normalizeSemanticQuery(req.Query))
	if isZeroVector(queryVector) {
		return []SemanticHit{}, nil
	}

	topChunks := rankSemanticChunks(index.chunks, queryVector, req.Limit)
	hits := make([]SemanticHit, 0, len(topChunks))
	for _, scored := range topChunks {
		hits = append(hits, SemanticHit{
			Repo:      index.repo.Name,
			File:      scored.chunk.File,
			StartLine: scored.chunk.StartLine,
			EndLine:   scored.chunk.EndLine,
			Snippet:   scored.chunk.Snippet,
			Score:     scored.score,
			Source:    "vector",
		})
	}
	return hits, nil
}

func (b *hashSemanticBackend) Status(_ context.Context) (*SemanticStatus, error) {
	return &SemanticStatus{
		Backend: "hash",
		Enabled: true,
		Ready:   true,
		Message: "in-memory hash semantic backend",
	}, nil
}

func (b *hashSemanticBackend) ensureIndex(ctx context.Context, repo Repo) (*hashSemanticIndex, error) {
	b.mu.RLock()
	cached := b.indexes[repo.Root]
	b.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	index, err := b.buildIndex(ctx, repo)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	if existing := b.indexes[repo.Root]; existing != nil {
		b.mu.Unlock()
		return existing, nil
	}
	b.indexes[repo.Root] = index
	b.mu.Unlock()
	return index, nil
}

func (b *hashSemanticBackend) buildIndex(ctx context.Context, repo Repo) (*hashSemanticIndex, error) {
	chunks, buildStats, err := b.service.buildChunks(ctx, repo)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("semantic index has no chunks for repo %q", repo.Name)
	}

	index := &hashSemanticIndex{
		repo:       repo,
		chunks:     make([]semanticChunk, 0, len(chunks)),
		fileChunks: make(map[string][]int),
		stats: SemanticStats{
			Repo:          repo.Name,
			Backend:       "hash",
			FilesSeen:     buildStats.filesSeen,
			FilesIndexed:  buildStats.filesIndexed,
			ChunksIndexed: 0,
			ChunksSkipped: buildStats.chunksSkipped,
			DurationMS:    buildStats.durationMS,
		},
	}

	for _, chunk := range chunks {
		vector := semanticVector(chunk.Content)
		if isZeroVector(vector) {
			index.stats.ChunksSkipped++
			continue
		}
		chunk.Vector = vector
		idx := len(index.chunks)
		index.fileChunks[canonicalResultPath(chunk.File)] = append(index.fileChunks[canonicalResultPath(chunk.File)], idx)
		index.chunks = append(index.chunks, chunk)
	}
	index.stats.ChunksIndexed = len(index.chunks)
	if len(index.chunks) == 0 {
		return nil, fmt.Errorf("semantic index has no vectorizable chunks for repo %q", repo.Name)
	}

	return index, nil
}

func rankSemanticChunks(chunks []semanticChunk, queryVector []float32, limit int) []scoredSemanticChunk {
	if limit <= 0 {
		limit = 10
	}
	scored := make([]scoredSemanticChunk, 0, len(chunks))
	for _, chunk := range chunks {
		score := cosineSimilarity(queryVector, chunk.Vector)
		if score < semanticMinSimilarity {
			continue
		}
		scored = append(scored, scoredSemanticChunk{chunk: chunk, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			if scored[i].chunk.File == scored[j].chunk.File {
				return scored[i].chunk.StartLine < scored[j].chunk.StartLine
			}
			return scored[i].chunk.File < scored[j].chunk.File
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}

func semanticScoreForResult(index *hashSemanticIndex, result SemanticHit, queryVector []float32) float64 {
	file := canonicalResultPath(result.File)
	chunkIndexes := index.fileChunks[file]
	if len(chunkIndexes) == 0 {
		return 0
	}

	best := 0.0
	for _, idx := range chunkIndexes {
		chunk := index.chunks[idx]
		if result.StartLine > 0 {
			if result.StartLine < chunk.StartLine-8 || result.StartLine > chunk.EndLine+8 {
				continue
			}
		}
		score := cosineSimilarity(queryVector, chunk.Vector)
		if score > best {
			best = score
		}
	}
	if best > 0 {
		return best
	}
	for _, idx := range chunkIndexes {
		score := cosineSimilarity(queryVector, index.chunks[idx].Vector)
		if score > best {
			best = score
		}
	}
	return best
}

func canonicalResultPath(file string) string {
	file = filepath.ToSlash(strings.TrimSpace(file))
	file = strings.TrimPrefix(file, "./")
	if file == "" {
		return file
	}
	return strings.TrimPrefix(path.Clean(file), "./")
}

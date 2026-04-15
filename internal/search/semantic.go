package search

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/404loopback/zukt/internal/zoekt"
)

const (
	searchModeLexical  = "lexical"
	searchModeSemantic = "semantic"

	semanticVectorSize       = 256
	semanticChunkLines       = 40
	semanticChunkOverlap     = 12
	semanticMaxFileSizeBytes = 1 << 20
	semanticHybridPoolFactor = 6
	semanticHybridAugment    = 3
	semanticRRFK             = 60.0
	semanticMinSimilarity    = 0.18
	semanticLineBonusMin     = 0.08
)

type semanticIndex struct {
	Repo         string
	Root         string
	BuiltAt      time.Time
	FilesIndexed int
	Chunks       []semanticChunk
	fileChunks   map[string][]int
}

type semanticChunk struct {
	File      string
	StartLine int
	EndLine   int
	Snippet   string
	Vector    []float32
}

type SemanticIndexStats struct {
	Repo         string `json:"repo"`
	Root         string `json:"root"`
	FilesIndexed int    `json:"files_indexed"`
	Chunks       int    `json:"chunks"`
	BuiltAt      string `json:"built_at"`
}

type scoredSemanticChunk struct {
	chunk semanticChunk
	score float64
}

type hybridCandidate struct {
	result       zoekt.SearchResult
	score        float64
	lexicalRank  int
	semanticRank int
}

func normalizeSearchMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return searchModeSemantic
	}
	switch mode {
	case searchModeLexical, searchModeSemantic:
		return mode
	default:
		return mode
	}
}

func (s *Service) PrepareSemanticIndex(ctx context.Context, repo string) (SemanticIndexStats, error) {
	root, resolvedRepo, err := s.resolveRepoRoot(repo)
	if err != nil {
		return SemanticIndexStats{}, err
	}

	index, err := s.buildSemanticIndex(ctx, root, resolvedRepo)
	if err != nil {
		return SemanticIndexStats{}, err
	}

	s.semanticMu.Lock()
	s.semantic[root] = index
	s.semanticMu.Unlock()

	return semanticStatsFromIndex(index), nil
}

func (s *Service) searchSemantic(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}
	if limit == 0 {
		limit = 10
	}
	if strings.TrimSpace(repo) == "" {
		return nil, fmt.Errorf("repo is required for semantic mode")
	}

	index, err := s.ensureSemanticIndex(ctx, repo)
	if err != nil {
		return nil, err
	}

	queryVector := semanticVector(normalizeSemanticQuery(query))
	if isZeroVector(queryVector) {
		return []zoekt.SearchResult{}, nil
	}

	topChunks := rankSemanticChunks(index, queryVector, limit)
	results := make([]zoekt.SearchResult, 0, len(topChunks))
	for _, scored := range topChunks {
		results = append(results, zoekt.SearchResult{
			Repo:    index.Repo,
			File:    scored.chunk.File,
			Line:    scored.chunk.StartLine,
			Snippet: scored.chunk.Snippet,
		})
	}
	return results, nil
}

func (s *Service) searchHybrid(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}
	if limit == 0 {
		limit = 10
	}

	lexicalPool := limit * semanticHybridPoolFactor
	if lexicalPool < limit {
		lexicalPool = limit
	}
	if lexicalPool > 800 {
		lexicalPool = 800
	}

	lexicalResults, err := s.searchLexical(ctx, query, repo, lexicalPool)
	if err != nil {
		return nil, err
	}
	if len(lexicalResults) == 0 {
		if strings.TrimSpace(repo) != "" {
			return s.searchSemantic(ctx, query, repo, limit)
		}
		return []zoekt.SearchResult{}, nil
	}
	if strings.TrimSpace(repo) == "" {
		if len(lexicalResults) > limit {
			return lexicalResults[:limit], nil
		}
		return lexicalResults, nil
	}

	index, err := s.ensureSemanticIndex(ctx, repo)
	if err != nil {
		if len(lexicalResults) > limit {
			return lexicalResults[:limit], nil
		}
		return lexicalResults, nil
	}

	queryVector := semanticVector(normalizeSemanticQuery(query))
	if isZeroVector(queryVector) {
		if len(lexicalResults) > limit {
			return lexicalResults[:limit], nil
		}
		return lexicalResults, nil
	}

	candidates := make(map[string]hybridCandidate, len(lexicalResults)+limit*semanticHybridAugment)
	for i, result := range lexicalResults {
		key := searchResultKey(result)
		score := rrfScore(i + 1)
		if semanticScore := semanticScoreForResult(index, result, queryVector); semanticScore >= semanticLineBonusMin {
			score += 0.15 * semanticScore
		}
		candidate := candidates[key]
		candidate.result = result
		candidate.score += score
		candidate.lexicalRank = i + 1
		candidates[key] = candidate
	}

	semanticTop := rankSemanticChunks(index, queryVector, limit*semanticHybridAugment)
	for i, scored := range semanticTop {
		if scored.score < semanticMinSimilarity {
			continue
		}
		result := zoekt.SearchResult{
			Repo:    index.Repo,
			File:    scored.chunk.File,
			Line:    scored.chunk.StartLine,
			Snippet: scored.chunk.Snippet,
		}
		key := searchResultKey(result)
		score := 0.35*rrfScore(i+1) + 0.65*scored.score
		candidate := candidates[key]
		candidate.result = result
		candidate.score += score
		candidate.semanticRank = i + 1
		candidates[key] = candidate
	}

	ranked := make([]hybridCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, candidate)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].result.File == ranked[j].result.File {
				return ranked[i].result.Line < ranked[j].result.Line
			}
			return ranked[i].result.File < ranked[j].result.File
		}
		return ranked[i].score > ranked[j].score
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]zoekt.SearchResult, 0, len(ranked))
	for _, candidate := range ranked {
		out = append(out, candidate.result)
	}
	return out, nil
}

func (s *Service) ensureSemanticIndex(ctx context.Context, repo string) (*semanticIndex, error) {
	root, resolvedRepo, err := s.resolveRepoRoot(repo)
	if err != nil {
		return nil, err
	}

	s.semanticMu.RLock()
	existing := s.semantic[root]
	s.semanticMu.RUnlock()
	if existing != nil {
		return existing, nil
	}

	index, err := s.buildSemanticIndex(ctx, root, resolvedRepo)
	if err != nil {
		return nil, err
	}

	s.semanticMu.Lock()
	// Re-check to avoid overriding work from another concurrent preparation.
	if cached := s.semantic[root]; cached != nil {
		s.semanticMu.Unlock()
		return cached, nil
	}
	s.semantic[root] = index
	s.semanticMu.Unlock()

	return index, nil
}

func (s *Service) buildSemanticIndex(ctx context.Context, root, resolvedRepo string) (*semanticIndex, error) {
	index := &semanticIndex{
		Repo:       resolvedRepo,
		Root:       root,
		BuiltAt:    time.Now().UTC(),
		Chunks:     make([]semanticChunk, 0, 1024),
		fileChunks: make(map[string][]int),
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			name := d.Name()
			if _, excluded := s.excludeDirs[name]; excluded && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.Size() <= 0 || info.Size() > semanticMaxFileSizeBytes {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if !isSemanticCandidateFile(rel) {
			return nil
		}

		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if !utf8.Valid(body) || isLikelyBinary(body) {
			return nil
		}

		lines := splitNormalizedLines(string(body))
		if len(lines) == 0 {
			return nil
		}

		countBefore := len(index.Chunks)
		for start := 1; start <= len(lines); {
			end := start + semanticChunkLines - 1
			if end > len(lines) {
				end = len(lines)
			}
			chunkLines := lines[start-1 : end]
			chunkText := strings.Join(chunkLines, "\n")
			vector := semanticVector(chunkText)
			if !isZeroVector(vector) {
				chunk := semanticChunk{
					File:      rel,
					StartLine: start,
					EndLine:   end,
					Snippet:   semanticSnippet(chunkLines),
					Vector:    vector,
				}
				index.fileChunks[rel] = append(index.fileChunks[rel], len(index.Chunks))
				index.Chunks = append(index.Chunks, chunk)
			}
			if end == len(lines) {
				break
			}
			nextStart := end - semanticChunkOverlap + 1
			if nextStart <= start {
				nextStart = start + 1
			}
			start = nextStart
		}

		if len(index.Chunks) > countBefore {
			index.FilesIndexed++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(index.Chunks) == 0 {
		return nil, fmt.Errorf("semantic index has no chunks for repo %q", resolvedRepo)
	}

	return index, nil
}

func semanticStatsFromIndex(index *semanticIndex) SemanticIndexStats {
	return SemanticIndexStats{
		Repo:         index.Repo,
		Root:         index.Root,
		FilesIndexed: index.FilesIndexed,
		Chunks:       len(index.Chunks),
		BuiltAt:      index.BuiltAt.Format(time.RFC3339),
	}
}

func semanticVector(text string) []float32 {
	tokens := tokenizeSemantic(text)
	vec := make([]float32, semanticVectorSize)
	if len(tokens) == 0 {
		return vec
	}

	for _, token := range tokens {
		hash := semanticHash(token)
		bucket := int(hash % semanticVectorSize)
		sign := float32(1)
		if hash&1 == 1 {
			sign = -1
		}
		vec[bucket] += sign
	}

	normalizeVector(vec)
	return vec
}

func normalizeVector(vec []float32) {
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm == 0 {
		return
	}
	denom := float32(math.Sqrt(norm))
	for i := range vec {
		vec[i] /= denom
	}
}

func isZeroVector(vec []float32) bool {
	for _, v := range vec {
		if v != 0 {
			return false
		}
	}
	return true
}

func cosineSimilarity(left, right []float32) float64 {
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	var dot float64
	for i := 0; i < n; i++ {
		dot += float64(left[i] * right[i])
	}
	return dot
}

func tokenizeSemantic(text string) []string {
	tokens := make([]string, 0, 64)
	var builder strings.Builder
	emit := func() {
		if builder.Len() == 0 {
			return
		}
		token := builder.String()
		builder.Reset()
		for _, expanded := range expandToken(token) {
			if semanticStopword(expanded) {
				continue
			}
			tokens = append(tokens, expanded)
		}
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			builder.WriteRune(r)
			continue
		}
		emit()
	}
	emit()
	return tokens
}

func expandToken(token string) []string {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	base := strings.ToLower(token)
	out := make([]string, 0, 4)
	if len(base) >= 2 {
		out = append(out, base)
	}

	for _, part := range strings.Split(base, "_") {
		part = strings.TrimSpace(part)
		if len(part) < 2 {
			continue
		}
		out = append(out, part)
	}
	return out
}

func semanticStopword(token string) bool {
	switch token {
	case "the", "and", "for", "with", "from", "this", "that", "then", "true", "false", "null", "none", "void",
		"func", "function", "class", "const", "let", "var", "def", "async", "await", "public", "private",
		"static", "string", "bool", "value", "data", "item", "json", "http":
		return true
	default:
		return false
	}
}

func semanticHash(token string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(token))
	return h.Sum32()
}

func semanticSnippet(lines []string) string {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if countAlphaNum(line) < 3 {
			continue
		}
		if len(line) > 240 {
			return line[:240]
		}
		return line
	}
	return ""
}

func splitNormalizedLines(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func isLikelyBinary(body []byte) bool {
	for _, b := range body {
		if b == 0 {
			return true
		}
	}
	return false
}

func isSemanticCandidateFile(rel string) bool {
	base := strings.ToLower(pathpkg.Base(rel))
	switch base {
	case "go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "poetry.lock":
		return false
	case "dockerfile", "makefile", "jenkinsfile":
		return true
	}

	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".vue", ".java", ".kt", ".rb", ".rs",
		".c", ".cc", ".cpp", ".h", ".hpp", ".cs", ".php", ".swift", ".m", ".mm",
		".scala", ".sql", ".sh", ".bash", ".zsh", ".ps1":
		return true
	default:
		return false
	}
}

func rankSemanticChunks(index *semanticIndex, queryVector []float32, limit int) []scoredSemanticChunk {
	if limit <= 0 {
		limit = 10
	}
	scored := make([]scoredSemanticChunk, 0, len(index.Chunks))
	for _, chunk := range index.Chunks {
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

func semanticScoreForResult(index *semanticIndex, result zoekt.SearchResult, queryVector []float32) float64 {
	file := canonicalResultPath(result.File)
	chunkIndexes := index.fileChunks[file]
	if len(chunkIndexes) == 0 {
		return 0
	}

	best := 0.0
	for _, idx := range chunkIndexes {
		chunk := index.Chunks[idx]
		if result.Line > 0 {
			if result.Line < chunk.StartLine-8 || result.Line > chunk.EndLine+8 {
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
		score := cosineSimilarity(queryVector, index.Chunks[idx].Vector)
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
	return strings.TrimPrefix(pathpkg.Clean(file), "./")
}

func rrfScore(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1.0 / (semanticRRFK + float64(rank))
}

func searchResultKey(result zoekt.SearchResult) string {
	return result.Repo + "\x00" + result.File + "\x00" + fmt.Sprintf("%d", result.Line) + "\x00" + result.Snippet
}

func normalizeSemanticQuery(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return query
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "r:"):
			continue
		case strings.HasPrefix(field, "case:"):
			continue
		case strings.HasPrefix(field, "lang:"):
			continue
		case strings.HasPrefix(field, "sym:"):
			value := strings.TrimPrefix(field, "sym:")
			if value != "" {
				out = append(out, value)
			}
		case strings.HasPrefix(field, "file:"):
			value := strings.TrimPrefix(field, "file:")
			if value != "" {
				out = append(out, value)
			}
		default:
			out = append(out, field)
		}
	}
	if len(out) == 0 {
		return query
	}
	return strings.Join(out, " ")
}

func countAlphaNum(value string) int {
	count := 0
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			count++
		}
	}
	return count
}

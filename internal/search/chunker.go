package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	semanticVectorSize    = 256
	semanticMinSimilarity = 0.18
)

type semanticChunk struct {
	ID        string
	Repo      string
	RepoPath  string
	File      string
	StartLine int
	EndLine   int
	Language  string
	ChunkHash string
	Content   string
	Snippet   string
	Vector    []float32
}

type chunkBuildStats struct {
	filesSeen     int
	filesIndexed  int
	chunksIndexed int
	chunksSkipped int
	durationMS    int64
}

func (s *Service) buildChunks(ctx context.Context, repo Repo) ([]semanticChunk, chunkBuildStats, error) {
	start := time.Now()
	chunks := make([]semanticChunk, 0, 1024)
	stats := chunkBuildStats{}

	err := filepath.WalkDir(repo.Root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			name := d.Name()
			if _, excluded := s.excludeDirs[name]; excluded && path != repo.Root {
				return filepath.SkipDir
			}
			return nil
		}

		stats.filesSeen++
		info, err := d.Info()
		if err != nil {
			stats.chunksSkipped++
			return nil
		}
		if info.Size() <= 0 || info.Size() > s.semanticRuntimeConfig.MaxFileBytes {
			stats.chunksSkipped++
			return nil
		}

		rel, err := filepath.Rel(repo.Root, path)
		if err != nil {
			stats.chunksSkipped++
			return nil
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if !isSemanticCandidateFile(rel) {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			stats.chunksSkipped++
			return nil
		}
		if !utf8.Valid(body) || isLikelyBinary(body) {
			stats.chunksSkipped++
			return nil
		}

		lines := splitNormalizedLines(string(body))
		if len(lines) == 0 {
			return nil
		}

		countBefore := len(chunks)
		for startLine := 1; startLine <= len(lines); {
			endLine := startLine + s.semanticRuntimeConfig.ChunkLines - 1
			if endLine > len(lines) {
				endLine = len(lines)
			}
			chunkLines := lines[startLine-1 : endLine]
			content := strings.Join(chunkLines, "\n")
			chunkHash := hashChunk(rel, startLine, endLine, content)
			chunks = append(chunks, semanticChunk{
				ID:        chunkHash,
				Repo:      repo.Name,
				RepoPath:  repo.Root,
				File:      rel,
				StartLine: startLine,
				EndLine:   endLine,
				Language:  inferLanguage(rel),
				ChunkHash: chunkHash,
				Content:   content,
				Snippet:   semanticSnippet(chunkLines),
			})
			if endLine == len(lines) {
				break
			}
			nextStart := endLine - s.semanticRuntimeConfig.ChunkOverlap + 1
			if nextStart <= startLine {
				nextStart = startLine + 1
			}
			startLine = nextStart
		}

		if len(chunks) > countBefore {
			stats.filesIndexed++
		}
		return nil
	})
	if err != nil {
		return nil, stats, err
	}

	stats.chunksIndexed = len(chunks)
	stats.durationMS = time.Since(start).Milliseconds()
	return chunks, stats, nil
}

func hashChunk(file string, startLine, endLine int, content string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(file))
	_, _ = h.Write([]byte("\n"))
	_, _ = h.Write([]byte(strings.TrimSpace(content)))
	_, _ = h.Write([]byte("\n"))
	_, _ = h.Write([]byte(fmt.Sprintf("%d:%d", startLine, endLine)))
	return hex.EncodeToString(h.Sum(nil))
}

func inferLanguage(file string) string {
	base := strings.ToLower(pathpkg.Base(file))
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	case "jenkinsfile":
		return "groovy"
	}

	ext := strings.ToLower(filepath.Ext(file))
	if strings.HasPrefix(ext, ".") {
		ext = ext[1:]
	}
	if ext == "" {
		return "text"
	}
	return ext
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
		".scala", ".sql", ".sh", ".bash", ".zsh", ".ps1", ".yaml", ".yml", ".json", ".toml", ".md":
		return true
	default:
		return false
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

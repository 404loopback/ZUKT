package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/404loopback/zukt/internal/paths"
	"github.com/404loopback/zukt/internal/zoekt"
)

type Service struct {
	searcher              zoekt.Searcher
	allowedDirs           map[string]struct{}
	allowedList           []string
	excludeDirs           map[string]struct{}
	semanticBackend       SemanticBackend
	semanticRuntimeConfig SemanticRuntimeConfig
}

func NewService(searcher zoekt.Searcher, allowedDirs []string, excludeDirs []string, opts ...ServiceOption) *Service {
	options := defaultServiceOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	allowed := make(map[string]struct{}, len(allowedDirs))
	allowedList := make([]string, 0, len(allowedDirs))
	for _, dir := range allowedDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		normalized := filepath.Clean(abs)
		if _, ok := allowed[normalized]; ok {
			continue
		}
		allowed[normalized] = struct{}{}
		allowedList = append(allowedList, normalized)
	}

	exclude := make(map[string]struct{}, len(excludeDirs))
	for _, name := range excludeDirs {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		exclude[name] = struct{}{}
	}

	svc := &Service{
		searcher:              searcher,
		allowedDirs:           allowed,
		allowedList:           allowedList,
		excludeDirs:           exclude,
		semanticRuntimeConfig: options.semanticConfig,
	}

	switch strings.ToLower(strings.TrimSpace(options.semanticConfig.Backend)) {
	case "", "hash":
		svc.semanticBackend = newHashSemanticBackend(svc)
	case "disabled":
		svc.semanticBackend = nil
	case "qdrant":
		backend, err := newQdrantSemanticBackend(svc, options.semanticConfig)
		if err != nil {
			svc.semanticBackend = newHashSemanticBackend(svc)
		} else {
			svc.semanticBackend = backend
		}
	default:
		svc.semanticBackend = newHashSemanticBackend(svc)
	}
	return svc
}

func (s *Service) SearchCode(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	return s.searchLexical(ctx, query, repo, limit)
}

func (s *Service) SearchCodeWithMode(ctx context.Context, query, repo string, limit int, mode string) ([]zoekt.SearchResult, error) {
	switch normalizeSearchMode(mode) {
	case searchModeLexical:
		return s.searchLexical(ctx, query, repo, limit)
	case searchModeSemantic:
		return s.searchSemanticMode(ctx, query, repo, limit)
	case searchModeHybrid:
		return s.searchHybridMode(ctx, query, repo, limit)
	default:
		return nil, unsupportedModeError(mode)
	}
}

func (s *Service) searchLexical(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}

	repo = strings.TrimSpace(repo)
	// Keep path-based validation for absolute-path repos while allowing logical names (eg "ZUKT").
	if repo != "" && filepath.IsAbs(repo) && !s.isWithinAllowedRoots(repo) {
		return nil, fmt.Errorf("repo %q is outside allowed roots", repo)
	}

	backendLimit := limit
	if limit > 0 {
		backendLimit = limit * 2
	}
	results, err := s.searcher.Search(ctx, query, repo, backendLimit)
	if err != nil {
		return nil, err
	}

	filtered := make([]zoekt.SearchResult, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		// When repo names are absolute paths, enforce the same local policy as file tools.
		if filepath.IsAbs(strings.TrimSpace(r.Repo)) && !s.isWithinAllowedRoots(r.Repo) {
			continue
		}
		// Filter by excluded directory names
		if s.shouldExcludePath(r.File) {
			continue
		}
		key := r.Repo + "\x00" + r.File + "\x00" + fmt.Sprintf("%d", r.Line) + "\x00" + r.Snippet
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, r)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}

	return filtered, nil
}

func (s *Service) ListRepos(ctx context.Context) ([]string, error) {
	return s.searcher.ListRepos(ctx)
}

func (s *Service) isWithinAllowedRoots(candidate string) bool {
	if len(s.allowedDirs) == 0 {
		return true
	}

	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	candidateAbs = filepath.Clean(candidateAbs)
	if resolved, err := filepath.EvalSymlinks(candidateAbs); err == nil {
		candidateAbs = filepath.Clean(resolved)
	}

	for _, allowed := range s.allowedList {
		allowedAbs := allowed
		if resolved, err := filepath.EvalSymlinks(allowedAbs); err == nil {
			allowedAbs = filepath.Clean(resolved)
		}

		within, relErr := paths.IsWithinRoot(candidateAbs, allowedAbs)
		if relErr == nil && within {
			return true
		}
	}
	return false
}

func (s *Service) shouldExcludePath(file string) bool {
	if len(s.excludeDirs) == 0 {
		return false
	}
	file = filepath.ToSlash(file)
	for _, segment := range strings.Split(file, "/") {
		if _, ok := s.excludeDirs[segment]; ok {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (s *Service) resolveRepo(_ context.Context, repo string) (Repo, error) {
	root, name, err := s.resolveRepoRoot(repo)
	if err != nil {
		return Repo{}, err
	}
	return Repo{Name: name, Root: root}, nil
}

package search

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/404loopback/zukt/internal/zoekt"
)

const (
	hybridPoolFactor = 6
	hybridAugment    = 3
	rrfK             = 60.0
)

type hybridCandidate struct {
	result       zoekt.SearchResult
	score        float64
	lexicalRank  int
	semanticRank int
}

func (s *Service) searchSemanticMode(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
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
	if s.semanticBackend == nil {
		return []zoekt.SearchResult{}, nil
	}

	repoRef, err := s.resolveRepo(ctx, repo)
	if err != nil {
		return nil, err
	}
	hits, err := s.semanticBackend.Search(ctx, SemanticSearchRequest{
		Repo:  repoRef,
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return semanticHitsToSearchResults(hits), nil
}

func (s *Service) searchHybridMode(ctx context.Context, query, repo string, limit int) ([]zoekt.SearchResult, error) {
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

	lexicalPool := limit * hybridPoolFactor
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
			return s.searchSemanticMode(ctx, query, repo, limit)
		}
		return []zoekt.SearchResult{}, nil
	}
	if strings.TrimSpace(repo) == "" || s.semanticBackend == nil {
		if len(lexicalResults) > limit {
			return lexicalResults[:limit], nil
		}
		return lexicalResults, nil
	}

	repoRef, err := s.resolveRepo(ctx, repo)
	if err != nil {
		if len(lexicalResults) > limit {
			return lexicalResults[:limit], nil
		}
		return lexicalResults, nil
	}

	queryVector := semanticVector(normalizeSemanticQuery(query))
	semanticHits, err := s.semanticBackend.Search(ctx, SemanticSearchRequest{
		Repo:  repoRef,
		Query: query,
		Limit: limit * hybridAugment,
	})
	if err != nil {
		if len(lexicalResults) > limit {
			return lexicalResults[:limit], nil
		}
		return lexicalResults, nil
	}

	candidates := make(map[string]hybridCandidate, len(lexicalResults)+len(semanticHits))
	for i, result := range lexicalResults {
		key := searchResultKey(result)
		candidate := candidates[key]
		candidate.result = result
		candidate.score += rrfScore(i + 1)
		candidate.lexicalRank = i + 1
		if isZeroVector(queryVector) {
			candidates[key] = candidate
			continue
		}
		if candidate.result.Source == "" {
			candidate.result.Source = "lexical"
		}
		candidates[key] = candidate
	}

	for i, hit := range semanticHits {
		result := semanticHitsToSearchResults([]SemanticHit{hit})[0]
		key := searchResultKey(result)
		candidate := candidates[key]
		candidate.result = result
		candidate.score += rrfScore(i+1) + (0.35 * hit.Score)
		candidate.semanticRank = i + 1
		if candidate.result.Source == "" {
			candidate.result.Source = "vector"
		}
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

func rrfScore(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1.0 / (rrfK + float64(rank))
}

func searchResultKey(result zoekt.SearchResult) string {
	return result.Repo + "\x00" + result.File + "\x00" + fmt.Sprintf("%d", result.Line) + "\x00" + result.Snippet
}

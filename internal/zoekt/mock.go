package zoekt

import "context"

type MockSearcher struct{}

func NewMockSearcher() *MockSearcher {
	return &MockSearcher{}
}

func (m *MockSearcher) Search(_ context.Context, query, repo string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	results := []SearchResult{
		{Repo: "example/repo", File: "main.go", Line: 12, Snippet: "func main() { /* " + query + " */ }"},
		{Repo: "example/repo", File: "internal/search/service.go", Line: 34, Snippet: "if repo == \"\" { return nil }"},
	}

	if repo != "" {
		filtered := make([]SearchResult, 0, len(results))
		for _, r := range results {
			if r.Repo == repo {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (m *MockSearcher) ListRepos(_ context.Context) ([]string, error) {
	return []string{"example/repo", "example/another-repo"}, nil
}

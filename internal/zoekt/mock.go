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
		{Repo: "zukt/mock-repo", File: "main.go", Line: 12, Snippet: "func main() { /* " + query + " */ }"},
		{Repo: "zukt/mock-repo", File: "internal/search/service.go", Line: 34, Snippet: "if repo == \"\" { return nil }"},
		{Repo: "zukt/mock-repo", File: "frontend/node_modules/x.js", Line: 1, Snippet: "module.exports = {}"},
		{Repo: "zukt/mock-repo", File: "backend/.venv/lib/y.py", Line: 2, Snippet: "def main():"},
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
	return []string{"zukt/mock-repo", "zukt/mock-repo-2"}, nil
}

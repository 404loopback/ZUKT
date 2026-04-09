package zoekt

import "context"

type SearchResult struct {
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type Searcher interface {
	Search(ctx context.Context, query, repo string, limit int) ([]SearchResult, error)
	ListRepos(ctx context.Context) ([]string, error)
}

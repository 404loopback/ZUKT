package zoekt

import "context"

type SearchResult struct {
	Repo    string  `json:"repo"`
	File    string  `json:"file"`
	Line    int     `json:"line"`
	EndLine int     `json:"end_line,omitempty"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score,omitempty"`
	Source  string  `json:"source,omitempty"`
}

type Searcher interface {
	Search(ctx context.Context, query, repo string, limit int) ([]SearchResult, error)
	ListRepos(ctx context.Context) ([]string, error)
}

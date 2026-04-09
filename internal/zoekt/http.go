package zoekt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

var searchEndpointCandidates = []string{
	"/api/search",
	"/search",
}

var reposEndpointCandidates = []string{
	"/api/repos",
	"/api/list",
}

type HTTPSearcher struct {
	baseURL     *url.URL
	client      *http.Client
	maxAttempts int
}

func NewHTTPSearcher(baseURL string, timeout time.Duration) (*HTTPSearcher, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL cannot be empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q", baseURL)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &HTTPSearcher{
		baseURL:     parsed,
		client:      &http.Client{Timeout: timeout},
		maxAttempts: 3,
	}, nil
}

func (h *HTTPSearcher) Search(ctx context.Context, query, repo string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 10
	}

	candidates := []string{query}
	escaped := regexp.QuoteMeta(query)
	if escaped != query {
		candidates = append(candidates, escaped)
	}

	var lastErr error
	for _, candidateQuery := range candidates {
		for _, endpoint := range searchEndpointCandidates {
			results, err := h.searchOnce(ctx, endpoint, candidateQuery, repo, limit)
			if err == nil {
				return results, nil
			}
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no search endpoint tried")
	}
	return nil, fmt.Errorf("zoekt search failed: %w", lastErr)
}

func (h *HTTPSearcher) ListRepos(ctx context.Context) ([]string, error) {
	var lastErr error
	for _, endpoint := range reposEndpointCandidates {
		repos, err := h.listReposOnce(ctx, endpoint)
		if err == nil {
			return repos, nil
		}
		lastErr = err
	}

	// Fallback: infer repositories from a small search request when dedicated endpoint is unavailable.
	results, err := h.Search(ctx, ".", "", 20)
	if err != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("list repos failed (%v), fallback search failed: %w", lastErr, err)
		}
		return nil, fmt.Errorf("list repos fallback search failed: %w", err)
	}

	seen := make(map[string]struct{})
	for _, r := range results {
		if r.Repo == "" {
			continue
		}
		seen[r.Repo] = struct{}{}
	}

	repos := make([]string, 0, len(seen))
	for repo := range seen {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	return repos, nil
}

func (h *HTTPSearcher) searchOnce(ctx context.Context, endpoint, query, repo string, limit int) ([]SearchResult, error) {
	params := url.Values{}
	params.Set("q", buildQuery(query, repo))
	params.Set("num", fmt.Sprintf("%d", limit))
	params.Set("format", "json")

	body, status, err := h.get(ctx, endpoint, params)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint not found: %s", endpoint)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("unexpected status %d on %s", status, endpoint)
	}

	results, err := parseSearchResults(body)
	if err != nil {
		return nil, fmt.Errorf("parse search response from %s: %w", endpoint, err)
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

func (h *HTTPSearcher) listReposOnce(ctx context.Context, endpoint string) ([]string, error) {
	body, status, err := h.get(ctx, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint not found: %s", endpoint)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("unexpected status %d on %s", status, endpoint)
	}

	repos, err := parseRepos(body)
	if err != nil {
		return nil, fmt.Errorf("parse repos response from %s: %w", endpoint, err)
	}
	sort.Strings(repos)
	return repos, nil
}

func (h *HTTPSearcher) get(ctx context.Context, endpoint string, params url.Values) ([]byte, int, error) {
	u := *h.baseURL
	u.Path = path.Join(h.baseURL.Path, endpoint)
	u.RawQuery = params.Encode()

	var lastErr error
	var lastStatus int
	for attempt := 1; attempt <= h.maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, 0, fmt.Errorf("build request: %w", err)
		}

		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			if !isRetryableError(err) || attempt == h.maxAttempts {
				return nil, 0, lastErr
			}
			time.Sleep(backoff(attempt))
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response body: %w", readErr)
			if attempt == h.maxAttempts {
				return nil, resp.StatusCode, lastErr
			}
			time.Sleep(backoff(attempt))
			continue
		}

		lastStatus = resp.StatusCode
		if resp.StatusCode >= 500 && attempt < h.maxAttempts {
			time.Sleep(backoff(attempt))
			continue
		}
		return body, resp.StatusCode, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed with status %d", lastStatus)
	}
	return nil, lastStatus, lastErr
}

func buildQuery(query, repo string) string {
	if strings.TrimSpace(repo) == "" {
		return query
	}
	return fmt.Sprintf("repo:%s %s", repo, query)
}

func parseSearchResults(body []byte) ([]SearchResult, error) {
	type rawPosition struct {
		LineNumber int `json:"LineNumber"`
		LineNum    int `json:"lineNumber"`
		Line       int `json:"line"`
	}
	type rawRange struct {
		Start rawPosition `json:"Start"`
		End   rawPosition `json:"End"`
	}
	type rawChunkMatch struct {
		Content        string     `json:"Content"`
		ContentLower   string     `json:"content"`
		Ranges         []rawRange `json:"Ranges"`
		RangesLower    []rawRange `json:"ranges"`
		ContentSnippet string     `json:"ContentSnippet"`
	}
	type rawMatch struct {
		Line       string `json:"Line"`
		LineLower  string `json:"line"`
		LineNumber int    `json:"LineNumber"`
		LineNum    int    `json:"lineNumber"`
	}
	type rawFileMatch struct {
		Repository      string          `json:"Repository"`
		RepositoryLower string          `json:"repository"`
		Repo            string          `json:"Repo"`
		FileName        string          `json:"FileName"`
		FileNameLower   string          `json:"fileName"`
		File            string          `json:"File"`
		FileLower       string          `json:"file"`
		Path            string          `json:"path"`
		LineMatches     []rawMatch      `json:"LineMatches"`
		LineMatchesLow  []rawMatch      `json:"lineMatches"`
		Matches         []rawMatch      `json:"Matches"`
		ChunkMatches    []rawChunkMatch `json:"ChunkMatches"`
		ChunkMatchesLow []rawChunkMatch `json:"chunkMatches"`
	}
	type searchPayload struct {
		FileMatches []rawFileMatch `json:"FileMatches"`
		Files       []rawFileMatch `json:"Files"`
	}
	type response struct {
		Result       searchPayload `json:"Result"`
		SearchResult searchPayload `json:"SearchResult"`
	}

	var top response
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, err
	}

	all := make([]rawFileMatch, 0, len(top.Result.FileMatches)+len(top.Result.Files)+len(top.SearchResult.FileMatches)+len(top.SearchResult.Files))
	all = append(all, top.Result.FileMatches...)
	all = append(all, top.Result.Files...)
	all = append(all, top.SearchResult.FileMatches...)
	all = append(all, top.SearchResult.Files...)

	if len(all) == 0 {
		// Attempt flat shape fallback.
		var flat struct {
			FileMatches []rawFileMatch `json:"FileMatches"`
			Files       []rawFileMatch `json:"Files"`
		}
		if err := json.Unmarshal(body, &flat); err == nil {
			all = append(all, flat.FileMatches...)
			all = append(all, flat.Files...)
		}
	}

	results := make([]SearchResult, 0)
	for _, fm := range all {
		repo := fm.Repository
		if repo == "" {
			repo = fm.RepositoryLower
		}
		if repo == "" {
			repo = fm.Repo
		}
		file := fm.FileName
		if file == "" {
			file = fm.FileNameLower
		}
		if file == "" {
			file = fm.File
		}
		if file == "" {
			file = fm.FileLower
		}
		if file == "" {
			file = fm.Path
		}

		lines := fm.LineMatches
		if len(lines) == 0 {
			lines = fm.LineMatchesLow
		}
		if len(lines) == 0 {
			lines = fm.Matches
		}
		if len(lines) == 0 {
			chunks := fm.ChunkMatches
			if len(chunks) == 0 {
				chunks = fm.ChunkMatchesLow
			}
			for _, c := range chunks {
				snippet := c.Content
				if snippet == "" {
					snippet = c.ContentLower
				}
				if snippet == "" {
					snippet = c.ContentSnippet
				}
				snippet = firstLine(snippet)

				ranges := c.Ranges
				if len(ranges) == 0 {
					ranges = c.RangesLower
				}
				lineNumber := 0
				if len(ranges) > 0 {
					lineNumber = ranges[0].Start.LineNumber
					if lineNumber == 0 {
						lineNumber = ranges[0].Start.LineNum
					}
					if lineNumber == 0 {
						lineNumber = ranges[0].Start.Line
					}
				}
				results = append(results, SearchResult{
					Repo:    repo,
					File:    file,
					Line:    lineNumber,
					Snippet: snippet,
				})
			}
		}
		if len(lines) == 0 && len(fm.ChunkMatches) == 0 && len(fm.ChunkMatchesLow) == 0 {
			results = append(results, SearchResult{
				Repo:    repo,
				File:    file,
				Line:    0,
				Snippet: "",
			})
			continue
		}
		for _, m := range lines {
			line := m.Line
			if line == "" {
				line = m.LineLower
			}
			lineNumber := m.LineNumber
			if lineNumber == 0 {
				lineNumber = m.LineNum
			}
			results = append(results, SearchResult{
				Repo:    repo,
				File:    file,
				Line:    lineNumber,
				Snippet: line,
			})
		}
	}
	return results, nil
}

func parseRepos(body []byte) ([]string, error) {
	// Common shapes:
	// {"repos":["a","b"]} or {"Repositories":[{"name":"a"}]} or {"RepoURLs":["a","b"]}
	var shape1 struct {
		Repos        []string `json:"repos"`
		Repositories []string `json:"repositories"`
		RepoURLs     []string `json:"RepoURLs"`
	}
	if err := json.Unmarshal(body, &shape1); err == nil {
		repos := uniqueStrings(append(append(shape1.Repos, shape1.Repositories...), shape1.RepoURLs...))
		if len(repos) > 0 {
			return repos, nil
		}
	}

	var shape2 struct {
		Repositories []struct {
			Name  string `json:"name"`
			NameU string `json:"Name"`
			Repo  string `json:"repo"`
			RepoU string `json:"Repo"`
		} `json:"Repositories"`
		Repos []struct {
			Name  string `json:"name"`
			NameU string `json:"Name"`
			Repo  string `json:"repo"`
			RepoU string `json:"Repo"`
		} `json:"repos"`
	}
	if err := json.Unmarshal(body, &shape2); err != nil {
		// Continue with generic payload traversal below.
	} else {
		repos := make([]string, 0, len(shape2.Repositories)+len(shape2.Repos))
		for _, r := range shape2.Repositories {
			name := firstNonEmpty(r.Name, r.NameU, r.Repo, r.RepoU)
			if name != "" {
				repos = append(repos, name)
			}
		}
		for _, r := range shape2.Repos {
			name := firstNonEmpty(r.Name, r.NameU, r.Repo, r.RepoU)
			if name != "" {
				repos = append(repos, name)
			}
		}

		repos = uniqueStrings(repos)
		if len(repos) > 0 {
			return repos, nil
		}
	}

	var generic any
	if err := json.Unmarshal(body, &generic); err != nil {
		return nil, err
	}
	repos := make([]string, 0, 16)
	var walk func(parentKey string, v any)
	walk = func(parentKey string, v any) {
		switch vv := v.(type) {
		case map[string]any:
			for k, child := range vv {
				lower := strings.ToLower(strings.TrimSpace(k))
				// Branch metadata often contains names unrelated to repositories.
				if lower == "branches" || lower == "branch" {
					continue
				}
				if lower == "repository" || lower == "repo" {
					if s, ok := child.(string); ok {
						repos = append(repos, s)
					}
				}
				if lower == "name" && (parentKey == "repository" || parentKey == "repo" || parentKey == "repositories" || parentKey == "repos") {
					if s, ok := child.(string); ok {
						repos = append(repos, s)
					}
				}
				walk(lower, child)
			}
		case []any:
			for _, child := range vv {
				walk(parentKey, child)
			}
		}
	}
	walk("", generic)
	repos = uniqueStrings(repos)
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found in payload")
	}
	return repos, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func firstLine(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if idx := strings.IndexByte(v, '\n'); idx >= 0 {
		return strings.TrimSpace(v[:idx])
	}
	return v
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func isRetryableError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "timeout")
}

func backoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 100 * time.Millisecond
	case 2:
		return 250 * time.Millisecond
	default:
		return 500 * time.Millisecond
	}
}
